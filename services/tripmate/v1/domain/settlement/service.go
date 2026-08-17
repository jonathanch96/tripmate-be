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
	if err := validateDate(tc, in.Date); err != nil {
		return nil, err
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
	status := domainsettlement.StatusApproved
	if tc.Trip.Settings.ApprovalRequiredSettlements {
		status = domainsettlement.StatusPending
	}
	now := s.deps.Clock().UTC()
	row := &domainsettlement.Settlement{ID: uuid.New(), TripID: tc.Trip.ID, FromUserID: from.UserID, ToUserID: to.UserID, Amount: in.Amount, Currency: in.Currency, Method: in.Method, Note: in.Note, ProofURL: in.ProofURL, Status: status, CreatedByUserID: actor.UserID, Version: 1, SettlementDate: date(in.Date), CreatedAt: now, UpdatedAt: now}
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

func (s *service) Update(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID, in UpdateInput) (*domainsettlement.Settlement, error) {
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
	if in.Amount != nil {
		if !in.Amount.IsPositive() {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		row.Amount = *in.Amount
	}
	if in.Currency != nil {
		currency := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if !money.IsSupportedCurrency(currency) || !tc.Trip.AllowsCurrency(currency) {
			return nil, apperror.New("INVALID_CURRENCY")
		}
		row.Currency = currency
	}
	if in.Method != nil {
		if *in.Method != domainsettlement.MethodCash && *in.Method != domainsettlement.MethodBankTransfer {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		row.Method = *in.Method
	}
	if in.Note != nil {
		row.Note = in.Note
	}
	if in.ProofURL != nil {
		row.ProofURL = in.ProofURL
	}
	if in.Date != nil {
		if err = validateDate(tc, *in.Date); err != nil {
			return nil, err
		}
		row.SettlementDate = date(*in.Date)
	}
	if row.Method == domainsettlement.MethodBankTransfer {
		to, toErr := s.deps.Participants.GetByTripAndUser(ctx, tc.Trip.ID, row.ToUserID)
		if toErr != nil {
			return nil, apperror.New("PARTICIPANT_NOT_FOUND")
		}
		if to.BankInfo != nil {
			row.BankName, row.BankAccountNumber, row.BankAccountHolder = ptr(to.BankInfo.BankName), ptr(to.BankInfo.AccountNumber), ptr(to.BankInfo.AccountHolder)
		}
	}
	// Approval is a review gate on the ledger, not on editability: editing a settlement that was
	// already decided invalidates that decision, so it goes back to pending for re-review rather
	// than silently keeping (or ignoring) a stale approval/rejection.
	if tc.Trip.Settings.ApprovalRequiredSettlements && (row.Status == domainsettlement.StatusApproved || row.Status == domainsettlement.StatusRejected) {
		row.Status = domainsettlement.StatusPending
		row.ApprovedByUserID, row.ApprovedAt, row.RejectedReason = nil, nil, nil
	}
	row.Version = in.Version
	err = s.deps.UOW.Do(ctx, func(tx context.Context) error {
		if _, e := s.deps.Repo.Update(tx, row); e != nil {
			return e
		}
		return s.emit(tx, "settlement.updated", actor.UserID, row)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Repo.GetByID(ctx, id)
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

func validateDate(tc tripctx.TripContext, value time.Time) error {
	day := date(value)
	if day.Before(date(tc.Trip.StartDate).AddDate(0, 0, -7)) || day.After(date(tc.Trip.EndDate).AddDate(0, 0, 7)) {
		return apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "date", Rule: "range", Message: "date must be within seven days of the trip"}})
	}
	return nil
}

func date(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
