package finalization

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	shared "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/shared"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	"time"
)

type BalanceService interface {
	Calculate(context.Context, tripctx.TripContext) (*domainbalance.Result, error)
	FinalSettlement(context.Context, tripctx.TripContext) (*domainbalance.FinalPlan, error)
}
type FXService interface {
	LockAll(context.Context, uuid.UUID) error
}
type TripRepository interface {
	SetFinalized(context.Context, uuid.UUID, bool) error
}
type OutboxRepository interface {
	Create(context.Context, *event.OutboxEvent) error
}
type Dependencies struct {
	Balances BalanceService
	FX       FXService
	Trips    TripRepository
	Outbox   OutboxRepository
	UOW      shared.UnitOfWork
	Clock    func() time.Time
}
type Service interface {
	Finalize(context.Context, identity.Identity, tripctx.TripContext) (*domainbalance.FinalPlan, error)
	Unfinalize(context.Context, identity.Identity, tripctx.TripContext) error
}
type service struct{ deps Dependencies }

func NewService(d Dependencies) Service {
	if d.Clock == nil {
		d.Clock = time.Now
	}
	return &service{d}
}
func (s *service) Finalize(ctx context.Context, actor identity.Identity, tc tripctx.TripContext) (*domainbalance.FinalPlan, error) {
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	if tc.Trip.IsFinalized {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	var plan *domainbalance.FinalPlan
	err := s.deps.UOW.Do(ctx, func(tx context.Context) error {
		result, e := s.deps.Balances.Calculate(tx, tc)
		if e != nil {
			return e
		}
		if result.Summary.PendingExpenseCount > 0 || result.Summary.PendingSettlementCount > 0 {
			return apperror.Newf("VALIDATION_FAILED", "Resolve pending items: %d expenses, %d settlements", result.Summary.PendingExpenseCount, result.Summary.PendingSettlementCount)
		}
		plan, e = s.deps.Balances.FinalSettlement(tx, tc)
		if e != nil {
			return e
		}
		if e := s.deps.FX.LockAll(tx, tc.Trip.ID); e != nil {
			return e
		}
		if e := s.deps.Trips.SetFinalized(tx, tc.Trip.ID, true); e != nil {
			return e
		}
		return s.emit(tx, "trip.finalized", actor.UserID, tc.Trip.ID, plan)
	})
	return plan, err
}
func (s *service) Unfinalize(ctx context.Context, actor identity.Identity, tc tripctx.TripContext) error {
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return apperror.New("PLANNER_ONLY")
	}
	if !tc.Trip.IsFinalized {
		return apperror.New("VALIDATION_FAILED")
	}
	return s.deps.UOW.Do(ctx, func(tx context.Context) error {
		if e := s.deps.Trips.SetFinalized(tx, tc.Trip.ID, false); e != nil {
			return e
		}
		return s.emit(tx, "trip.unfinalized", actor.UserID, tc.Trip.ID, nil)
	})
}
func (s *service) emit(ctx context.Context, kind string, actor, tripID uuid.UUID, plan *domainbalance.FinalPlan) error {
	payload, err := json.Marshal(map[string]any{"trip_id": tripID, "actor_id": actor, "plan": plan})
	if err != nil {
		return err
	}
	now := s.deps.Clock().UTC()
	return s.deps.Outbox.Create(ctx, &event.OutboxEvent{ID: uuid.New(), AggregateType: "trip", AggregateID: tripID, EventType: kind, Payload: payload, Status: "pending", AvailableAt: now, CreatedAt: now})
}
