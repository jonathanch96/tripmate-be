package finalization

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	shared "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/shared"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
)

type balanceStub struct {
	result *domainbalance.Result
	err    error
}

func (s balanceStub) Calculate(context.Context, tripctx.TripContext) (*domainbalance.Result, error) {
	return s.result, s.err
}
func (s balanceStub) FinalSettlement(context.Context, tripctx.TripContext) (*domainbalance.FinalPlan, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domainbalance.FinalPlan{BaseCurrency: s.result.BaseCurrency, Transfers: s.result.Debts}, nil
}

type fxStub struct{ locks int }

func (s *fxStub) LockAll(context.Context, uuid.UUID) error { s.locks++; return nil }

type tripStub struct{ states []bool }

func (s *tripStub) SetFinalized(_ context.Context, _ uuid.UUID, state bool) error {
	s.states = append(s.states, state)
	return nil
}

type outboxStub struct{ rows []event.OutboxEvent }

func (s *outboxStub) Create(_ context.Context, row *event.OutboxEvent) error {
	s.rows = append(s.rows, *row)
	return nil
}

type directUOW struct{}

func (directUOW) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

var _ shared.UnitOfWork = directUOW{}

func finalFixture(pendingExpenses, pendingSettlements int) (Service, *fxStub, *tripStub, *outboxStub, identity.Identity, tripctx.TripContext) {
	fx, trips, events := &fxStub{}, &tripStub{}, &outboxStub{}
	actor := identity.Identity{UserID: uuid.New()}
	tc := tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New()}, Participant: domainparticipant.Participant{UserID: actor.UserID, Role: domainparticipant.RolePlanner}}
	result := &domainbalance.Result{BaseCurrency: "PHP", Summary: domainbalance.Summary{PendingExpenseCount: pendingExpenses, PendingSettlementCount: pendingSettlements}}
	service := NewService(Dependencies{Balances: balanceStub{result: result}, FX: fx, Trips: trips, Outbox: events, UOW: directUOW{}, Clock: func() time.Time { return time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC) }})
	return service, fx, trips, events, actor, tc
}

func TestFinalizeLocksRatesChangesTripAndSnapshotsPlan(t *testing.T) {
	service, fx, trips, events, actor, tc := finalFixture(0, 0)
	plan, err := service.Finalize(context.Background(), actor, tc)
	if err != nil {
		t.Fatal(err)
	}
	if plan.BaseCurrency != "PHP" || fx.locks != 1 || len(trips.states) != 1 || !trips.states[0] {
		t.Fatalf("plan=%+v locks=%d states=%v", plan, fx.locks, trips.states)
	}
	if len(events.rows) != 1 || events.rows[0].EventType != "trip.finalized" {
		t.Fatalf("events=%+v", events.rows)
	}
}

func TestFinalizeRejectsPendingItemsBeforeMutation(t *testing.T) {
	service, fx, trips, _, actor, tc := finalFixture(1, 2)
	_, err := service.Finalize(context.Background(), actor, tc)
	if !apperror.Is(err, "VALIDATION_FAILED") || fx.locks != 0 || len(trips.states) != 0 {
		t.Fatalf("err=%v locks=%d states=%v", err, fx.locks, trips.states)
	}
}

func TestUnfinalizeKeepsLockedRates(t *testing.T) {
	service, fx, trips, events, actor, tc := finalFixture(0, 0)
	tc.Trip.IsFinalized = true
	if err := service.Unfinalize(context.Background(), actor, tc); err != nil {
		t.Fatal(err)
	}
	if fx.locks != 0 || len(trips.states) != 1 || trips.states[0] || events.rows[0].EventType != "trip.unfinalized" {
		t.Fatalf("locks=%d states=%v events=%+v", fx.locks, trips.states, events.rows)
	}
}
