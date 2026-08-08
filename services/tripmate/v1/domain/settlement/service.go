package settlement

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
)

type service struct{ deps Dependencies }

func NewService(deps Dependencies) Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &service{deps: deps}
}

func (s *service) Record(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, in RecordInput) (*domainsettlement.Settlement, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	if in.FromUserID == in.ToUserID || !in.Amount.IsPositive() {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	if !tc.Trip.Settings.AllowSettlementBeforeEnd && s.deps.Clock().UTC().Before(tc.Trip.EndDate.UTC()) {
		return nil, apperror.New("SETTLEMENT_NOT_ALLOWED_YET")
	}
	from, err := s.deps.Participants.GetByTripAndUser(ctx, tc.Trip.ID, in.FromUserID)
	if err != nil {
		return nil, apperror.New("PARTICIPANT_NOT_FOUND")
	}
	to, err := s.deps.Participants.GetByTripAndUser(ctx, tc.Trip.ID, in.ToUserID)
	if err != nil {
		return nil, apperror.New("PARTICIPANT_NOT_FOUND")
	}
	if actor.UserID != in.FromUserID && actor.UserID != in.ToUserID && tc.Participant.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("FORBIDDEN")
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if !money.IsSupportedCurrency(in.Currency) || !tc.Trip.AllowsCurrency(in.Currency) {
		return nil, apperror.New("INVALID_CURRENCY")
	}
	if in.Method != domainsettlement.MethodCash && in.Method != domainsettlement.MethodBankTransfer {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	table, err := s.deps.FX.EffectiveTable(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	baseAmount, err := table.Convert(in.Amount, in.Currency, tc.Trip.BaseCurrency)
	if err != nil {
		return nil, err
	}
	outstanding, err := s.deps.Balances.OutstandingDebt(ctx, tc.Trip.ID, in.FromUserID, in.ToUserID)
	if err != nil {
		return nil, err
	}
	if baseAmount.GreaterThan(outstanding.Add(money.Epsilon)) {
		return nil, apperror.Newf("SETTLEMENT_EXCEEDS_DEBT", "Settlement exceeds outstanding debt of %s %s", outstanding.String(), tc.Trip.BaseCurrency)
	}
	status := domainsettlement.StatusApproved
	if tc.Trip.Settings.ApprovalRequiredSettlements {
		status = domainsettlement.StatusPending
	}
	now := s.deps.Clock().UTC()
	row := &domainsettlement.Settlement{ID: uuid.New(), TripID: tc.Trip.ID, FromUserID: from.UserID, ToUserID: to.UserID, Amount: in.Amount, Currency: in.Currency, Method: in.Method, Note: in.Note, ProofURL: in.ProofURL, Status: status, CreatedByUserID: actor.UserID, Version: 1, CreatedAt: now, UpdatedAt: now}
	if in.Method == domainsettlement.MethodBankTransfer && to.BankInfo != nil {
		row.BankName, row.BankAccountNumber, row.BankAccountHolder = ptr(to.BankInfo.BankName), ptr(to.BankInfo.AccountNumber), ptr(to.BankInfo.AccountHolder)
	}
	err = s.deps.UOW.Do(ctx, func(tx context.Context) error {
		if _, e := s.deps.Repo.Create(tx, row); e != nil {
			return e
		}
		return s.emit(tx, "settlement.created", actor.UserID, row)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Repo.GetByID(ctx, row.ID)
}

func (s *service) Approve(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID) (*domainsettlement.Settlement, error) {
	return s.transition(ctx, actor, tc, id, domainsettlement.StatusApproved, "")
}
func (s *service) Reject(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID, reason string) (*domainsettlement.Settlement, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	return s.transition(ctx, actor, tc, id, domainsettlement.StatusRejected, strings.TrimSpace(reason))
}
func (s *service) transition(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID, status domainsettlement.Status, reason string) (*domainsettlement.Settlement, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	row, err := s.owned(ctx, tc, id)
	if err != nil {
		return nil, err
	}
	if row.Status != domainsettlement.StatusPending {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	now := s.deps.Clock().UTC()
	row.Status = status
	if status == domainsettlement.StatusApproved {
		row.ApprovedByUserID = &actor.UserID
		row.ApprovedAt = &now
		row.RejectedReason = nil
	} else {
		row.RejectedReason = &reason
		row.ApprovedByUserID = nil
		row.ApprovedAt = nil
	}
	err = s.deps.UOW.Do(ctx, func(tx context.Context) error {
		if _, e := s.deps.Repo.Update(tx, row); e != nil {
			return e
		}
		return s.emit(tx, "settlement."+string(status), actor.UserID, row)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Repo.GetByID(ctx, id)
}
func (s *service) List(ctx context.Context, tc tripctx.TripContext, f Filter) ([]domainsettlement.Settlement, int64, error) {
	return s.deps.Repo.ListByTripID(ctx, tc.Trip.ID, f)
}
func (s *service) Delete(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID) error {
	if err := tc.Trip.AssertMutable(); err != nil {
		return err
	}
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return apperror.New("PLANNER_ONLY")
	}
	row, err := s.owned(ctx, tc, id)
	if err != nil {
		return err
	}
	if row.Status == domainsettlement.StatusApproved {
		return apperror.New("VALIDATION_FAILED")
	}
	return s.deps.UOW.Do(ctx, func(tx context.Context) error {
		if e := s.deps.Repo.SoftDelete(tx, id); e != nil {
			return e
		}
		return s.emit(tx, "settlement.deleted", actor.UserID, row)
	})
}
func (s *service) owned(ctx context.Context, tc tripctx.TripContext, id uuid.UUID) (*domainsettlement.Settlement, error) {
	row, err := s.deps.Repo.GetByID(ctx, id)
	if err != nil || row.TripID != tc.Trip.ID {
		return nil, apperror.New("SETTLEMENT_NOT_FOUND")
	}
	return row, nil
}
func (s *service) emit(ctx context.Context, kind string, actor uuid.UUID, row *domainsettlement.Settlement) error {
	payload, err := json.Marshal(map[string]any{"settlement_id": row.ID, "trip_id": row.TripID, "from_user_id": row.FromUserID, "to_user_id": row.ToUserID, "amount": row.Amount.String(), "currency": row.Currency, "actor_id": actor})
	if err != nil {
		return err
	}
	return s.deps.Outbox.Create(ctx, &event.OutboxEvent{ID: uuid.New(), AggregateType: "settlement", AggregateID: row.ID, EventType: kind, Payload: payload, Status: "pending", AvailableAt: s.deps.Clock().UTC(), CreatedAt: s.deps.Clock().UTC()})
}
func ptr(value string) *string { return &value }
