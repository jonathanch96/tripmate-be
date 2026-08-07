package settlement

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	"github.com/shopspring/decimal"
)

type memoryRepo struct{ row *domainsettlement.Settlement }

func (r *memoryRepo) Create(_ context.Context, row *domainsettlement.Settlement) (*domainsettlement.Settlement, error) {
	copy := *row
	r.row = &copy
	return &copy, nil
}
func (r *memoryRepo) GetByID(context.Context, uuid.UUID) (*domainsettlement.Settlement, error) {
	if r.row == nil {
		return nil, apperror.New("SETTLEMENT_NOT_FOUND")
	}
	copy := *r.row
	return &copy, nil
}
func (r *memoryRepo) Update(_ context.Context, row *domainsettlement.Settlement) (*domainsettlement.Settlement, error) {
	copy := *row
	copy.Version++
	r.row = &copy
	return &copy, nil
}
func (r *memoryRepo) ListByTripID(context.Context, uuid.UUID, Filter) ([]domainsettlement.Settlement, int64, error) {
	if r.row == nil {
		return nil, 0, nil
	}
	return []domainsettlement.Settlement{*r.row}, 1, nil
}
func (r *memoryRepo) ListForBalance(context.Context, uuid.UUID) ([]domainsettlement.Settlement, error) {
	return nil, nil
}
func (r *memoryRepo) SoftDelete(context.Context, uuid.UUID) error { r.row = nil; return nil }

type parts map[uuid.UUID]domainparticipant.Participant

func (p parts) GetByTripAndUser(_ context.Context, _ uuid.UUID, id uuid.UUID) (*domainparticipant.Participant, error) {
	row, ok := p[id]
	if !ok {
		return nil, apperror.New("PARTICIPANT_NOT_FOUND")
	}
	return &row, nil
}

type debts struct{ amount decimal.Decimal }

func (d debts) OutstandingDebt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (decimal.Decimal, error) {
	return d.amount, nil
}

type fxProvider struct{ table *fxdomain.RateTable }

func (f fxProvider) EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error) {
	return f.table, nil
}

type outbox struct{ rows []event.OutboxEvent }

func (o *outbox) Create(_ context.Context, row *event.OutboxEvent) error {
	o.rows = append(o.rows, *row)
	return nil
}

type uow struct{}

func (uow) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func fixture(approval bool) (Service, *memoryRepo, *outbox, identity.Identity, tripctx.TripContext, uuid.UUID, uuid.UUID) {
	planner, from, to, tripID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo, events := &memoryRepo{}, &outbox{}
	ps := parts{from: {UserID: from}, to: {UserID: to, BankInfo: &domainparticipant.BankInfo{BankName: "Bank", AccountNumber: "12345678", AccountHolder: "To"}}}
	tc := tripctx.TripContext{Trip: domaintrip.Trip{ID: tripID, BaseCurrency: "PHP", EndDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Settings: domaintrip.Settings{ApprovalRequiredSettlements: approval, AllowSettlementBeforeEnd: true}}, Participant: domainparticipant.Participant{UserID: planner, Role: domainparticipant.RolePlanner}}
	svc := NewService(Dependencies{Repo: repo, Participants: ps, Balances: debts{decimal.NewFromInt(100)}, FX: fxProvider{fxdomain.NewRateTable([]domainfx.Rate{{FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.NewFromInt(50)}})}, Outbox: events, UOW: uow{}, Clock: func() time.Time { return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC) }})
	return svc, repo, events, identity.Identity{UserID: planner}, tc, from, to
}

func TestRecordAppliesApprovalBankSnapshotAndOutbox(t *testing.T) {
	svc, _, events, actor, tc, from, to := fixture(true)
	row, err := svc.Record(context.Background(), actor, tc, RecordInput{FromUserID: from, ToUserID: to, Amount: decimal.NewFromInt(50), Currency: "php", Method: domainsettlement.MethodBankTransfer})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != domainsettlement.StatusPending || row.BankAccountNumber == nil || *row.BankAccountNumber != "12345678" || len(events.rows) != 1 {
		t.Fatalf("record=%+v events=%d", row, len(events.rows))
	}
}
func TestRecordGuards(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RecordInput, *tripctx.TripContext, *identity.Identity)
		code   string
	}{
		{"same user", func(in *RecordInput, _ *tripctx.TripContext, _ *identity.Identity) { in.ToUserID = in.FromUserID }, "VALIDATION_FAILED"},
		{"before end", func(_ *RecordInput, tc *tripctx.TripContext, _ *identity.Identity) {
			tc.Trip.Settings.AllowSettlementBeforeEnd = false
			tc.Trip.EndDate = time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
		}, "SETTLEMENT_NOT_ALLOWED_YET"},
		{"unrelated actor", func(_ *RecordInput, tc *tripctx.TripContext, a *identity.Identity) {
			tc.Participant.Role = domainparticipant.RoleParticipant
			a.UserID = uuid.New()
		}, "FORBIDDEN"},
		{"missing participant", func(in *RecordInput, _ *tripctx.TripContext, _ *identity.Identity) { in.ToUserID = uuid.New() }, "PARTICIPANT_NOT_FOUND"},
		{"invalid currency", func(in *RecordInput, tc *tripctx.TripContext, _ *identity.Identity) {
			in.Currency = "USD"
			tc.Trip.Settings.MultiCurrencyEnabled = false
		}, "INVALID_CURRENCY"},
		{"over settlement", func(in *RecordInput, _ *tripctx.TripContext, _ *identity.Identity) {
			in.Amount = decimal.NewFromInt(101)
		}, "SETTLEMENT_EXCEEDS_DEBT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, _, _, actor, tc, from, to := fixture(false)
			in := RecordInput{FromUserID: from, ToUserID: to, Amount: decimal.NewFromInt(50), Currency: "PHP", Method: domainsettlement.MethodCash}
			test.mutate(&in, &tc, &actor)
			_, err := svc.Record(context.Background(), actor, tc, in)
			if !apperror.Is(err, test.code) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestApproveRejectDeleteStateRules(t *testing.T) {
	svc, repo, _, actor, tc, from, to := fixture(true)
	created, err := svc.Record(context.Background(), actor, tc, RecordInput{FromUserID: from, ToUserID: to, Amount: decimal.NewFromInt(50), Currency: "PHP", Method: domainsettlement.MethodCash})
	if err != nil {
		t.Fatal(err)
	}
	member := actor
	tc.Participant.Role = domainparticipant.RoleParticipant
	if _, err = svc.Approve(context.Background(), member, tc, created.ID); !apperror.Is(err, "PLANNER_ONLY") {
		t.Fatalf("member approve=%v", err)
	}
	tc.Participant.Role = domainparticipant.RolePlanner
	rejected, err := svc.Reject(context.Background(), actor, tc, created.ID, "duplicate")
	if err != nil || rejected.Status != domainsettlement.StatusRejected {
		t.Fatalf("reject=%+v %v", rejected, err)
	}
	if err = svc.Delete(context.Background(), actor, tc, created.ID); err != nil || repo.row != nil {
		t.Fatalf("delete=%v row=%+v", err, repo.row)
	}
}
