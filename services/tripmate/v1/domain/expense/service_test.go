package expense

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	"github.com/shopspring/decimal"
)

type memoryExpenses struct{ row *domainexpense.Expense }

func (m *memoryExpenses) Create(_ context.Context, row *domainexpense.Expense) (*domainexpense.Expense, error) {
	copy := *row
	m.row = &copy
	return &copy, nil
}
func (m *memoryExpenses) GetByID(context.Context, uuid.UUID) (*domainexpense.Expense, error) {
	if m.row == nil {
		return nil, apperror.New("EXPENSE_NOT_FOUND")
	}
	copy := *m.row
	return &copy, nil
}
func (m *memoryExpenses) ListByTripID(context.Context, uuid.UUID, Filter) ([]domainexpense.Expense, int64, Totals, error) {
	if m.row == nil {
		return nil, 0, Totals{}, nil
	}
	return []domainexpense.Expense{*m.row}, 1, Totals{}, nil
}
func (m *memoryExpenses) ListApprovedByTripID(context.Context, uuid.UUID) ([]domainexpense.Expense, error) {
	return nil, nil
}
func (m *memoryExpenses) Update(_ context.Context, row *domainexpense.Expense) (*domainexpense.Expense, error) {
	copy := *row
	copy.Version++
	m.row = &copy
	return &copy, nil
}
func (m *memoryExpenses) SoftDelete(context.Context, uuid.UUID) error { m.row = nil; return nil }
func (m *memoryExpenses) CountByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *memoryExpenses) DistinctCurrencies(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}

type memoryPayers struct{}

func (memoryPayers) BulkCreate(context.Context, uuid.UUID, []domainexpense.Payer) error { return nil }
func (memoryPayers) ListByExpenseIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]domainexpense.Payer, error) {
	return nil, nil
}
func (memoryPayers) DeleteByExpenseID(context.Context, uuid.UUID) error { return nil }

type memorySplits struct{}

func (memorySplits) BulkCreate(context.Context, uuid.UUID, []domainexpense.Split) error { return nil }
func (memorySplits) ListByExpenseIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]domainexpense.Split, error) {
	return nil, nil
}
func (memorySplits) DeleteByExpenseID(context.Context, uuid.UUID) error { return nil }

type memoryParticipants []domainparticipant.Participant

func (m memoryParticipants) ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error) {
	return m, nil
}

type memoryOutbox struct{ rows []event.OutboxEvent }

func (m *memoryOutbox) Create(_ context.Context, row *event.OutboxEvent) error {
	m.rows = append(m.rows, *row)
	return nil
}

type directUOW struct{}

func (directUOW) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func expenseFixture(approval bool, permission domaintrip.EditPermission, role domainparticipant.Role) (*service, *memoryExpenses, *memoryOutbox, identity.Identity, tripctx.TripContext, uuid.UUID) {
	actor, other, tripID := uuid.New(), uuid.New(), uuid.New()
	tc := tripctx.TripContext{Trip: domaintrip.Trip{ID: tripID, Code: "ABC123", BaseCurrency: "PHP", StartDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), Settings: domaintrip.Settings{EditPermission: permission, ApprovalRequiredExpenses: approval}}, Participant: domainparticipant.Participant{TripID: tripID, UserID: actor, Role: role}}
	repo, outbox := &memoryExpenses{}, &memoryOutbox{}
	participants := memoryParticipants{{TripID: tripID, UserID: actor}, {TripID: tripID, UserID: other}}
	svc := NewService(Dependencies{Expenses: repo, Payers: memoryPayers{}, Splits: memorySplits{}, Participants: participants, Outbox: outbox, UOW: directUOW{}, Clock: func() time.Time { return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC) }}).(*service)
	return svc, repo, outbox, identity.Identity{UserID: actor}, tc, other
}

func TestCreateAppliesApprovalCurrencyDateAndOutboxRules(t *testing.T) {
	svc, _, outbox, actor, tc, other := expenseFixture(true, domaintrip.EditOwnOnly, domainparticipant.RoleParticipant)
	input := CreateInput{ExpenseDate: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Description: "Dinner", Amount: decimal.RequireFromString("100.00"), Currency: "php", SplitType: domainexpense.SplitEqual, Payers: []domainexpense.Payer{{UserID: actor.UserID, Amount: decimal.RequireFromString("100.00")}}, Participants: []uuid.UUID{actor.UserID, other}}
	created, err := svc.Create(context.Background(), actor, tc, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != domainexpense.StatusPending || len(created.Payers) != 1 || len(created.Splits) != 2 || len(outbox.rows) != 1 || outbox.rows[0].EventType != "expense.created" {
		t.Fatalf("created = %+v outbox=%+v", created, outbox.rows)
	}
	input.Currency = "USD"
	if _, err = svc.Create(context.Background(), actor, tc, input); !apperror.Is(err, "INVALID_CURRENCY") {
		t.Fatalf("currency error = %v", err)
	}
	input.Currency, input.ExpenseDate = "PHP", tc.Trip.StartDate.AddDate(0, 0, -8)
	if _, err = svc.Create(context.Background(), actor, tc, input); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("date error = %v", err)
	}
}

func TestEditPermissionMatrix(t *testing.T) {
	for _, operation := range []string{"update", "delete"} {
		for _, permission := range []domaintrip.EditPermission{domaintrip.EditEveryone, domaintrip.EditOwnOnly} {
			for _, subject := range []string{"creator", "other", "planner"} {
				t.Run(operation+"/"+string(permission)+"/"+subject, func(t *testing.T) {
					role := domainparticipant.RoleParticipant
					if subject == "planner" {
						role = domainparticipant.RolePlanner
					}
					svc, repo, _, actor, tc, other := expenseFixture(true, permission, role)
					creator := actor.UserID
					if subject == "other" {
						creator = other
					}
					repo.row = &domainexpense.Expense{ID: uuid.New(), TripID: tc.Trip.ID, ExpenseDate: tc.Trip.StartDate, Description: "Dinner", Amount: decimal.NewFromInt(10), Currency: "PHP", SplitType: domainexpense.SplitEqual, Status: domainexpense.StatusPending, CreatedByUserID: creator, Version: 1}
					var err error
					if operation == "update" {
						description := "Changed"
						_, err = svc.Update(context.Background(), actor, tc, repo.row.ID, UpdateInput{Description: &description, Version: 1})
					} else {
						err = svc.Delete(context.Background(), actor, tc, repo.row.ID)
					}
					allowed := permission == domaintrip.EditEveryone || subject != "other"
					if allowed && err != nil {
						t.Fatalf("expected allowed: %v", err)
					}
					if !allowed && !apperror.Is(err, "EDIT_OWN_ONLY") {
						t.Fatalf("expected EDIT_OWN_ONLY: %v", err)
					}
				})
			}
		}
	}
}

func TestApprovalStateMachineAndRejectedEdit(t *testing.T) {
	svc, repo, outbox, actor, tc, _ := expenseFixture(true, domaintrip.EditOwnOnly, domainparticipant.RolePlanner)
	repo.row = &domainexpense.Expense{ID: uuid.New(), TripID: tc.Trip.ID, ExpenseDate: tc.Trip.StartDate, Description: "Dinner", Amount: decimal.NewFromInt(10), Currency: "PHP", SplitType: domainexpense.SplitEqual, Status: domainexpense.StatusPending, CreatedByUserID: actor.UserID, Version: 1}
	approved, err := svc.Approve(context.Background(), actor, tc, repo.row.ID)
	if err != nil || approved.Status != domainexpense.StatusApproved || approved.ApprovedByUserID == nil {
		t.Fatalf("approve = %+v err=%v", approved, err)
	}
	if _, err = svc.Approve(context.Background(), actor, tc, repo.row.ID); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("second approve error = %v", err)
	}
	repo.row.Status, repo.row.Version = domainexpense.StatusPending, 3
	rejected, err := svc.Reject(context.Background(), actor, tc, repo.row.ID, "duplicate")
	if err != nil || rejected.Status != domainexpense.StatusRejected {
		t.Fatalf("reject = %+v err=%v", rejected, err)
	}
	description := "Corrected"
	edited, err := svc.Update(context.Background(), actor, tc, repo.row.ID, UpdateInput{Description: &description, Version: repo.row.Version})
	if err != nil || edited.Status != domainexpense.StatusPending || edited.RejectedReason != nil {
		t.Fatalf("edit rejected = %+v err=%v", edited, err)
	}
	if len(outbox.rows) != 3 {
		t.Fatalf("outbox rows = %d", len(outbox.rows))
	}
}

func TestCreateCoversMutableApprovalCurrencyAndDateBoundaries(t *testing.T) {
	svc, _, _, actor, tc, other := expenseFixture(false, domaintrip.EditOwnOnly, domainparticipant.RoleParticipant)
	input := CreateInput{ExpenseDate: tc.Trip.StartDate.AddDate(0, 0, -7), Description: "Hotel", Amount: decimal.NewFromInt(100), Currency: "PHP", SplitType: domainexpense.SplitEqual, Payers: []domainexpense.Payer{{UserID: actor.UserID, Amount: decimal.NewFromInt(100)}}, Participants: []uuid.UUID{actor.UserID, other}}
	created, err := svc.Create(context.Background(), actor, tc, input)
	if err != nil || created.Status != domainexpense.StatusApproved {
		t.Fatalf("Create(approval off, lower boundary) = %+v, %v", created, err)
	}
	input.ExpenseDate = tc.Trip.EndDate.AddDate(0, 0, 7)
	if _, err = svc.Create(context.Background(), actor, tc, input); err != nil {
		t.Fatalf("Create(upper boundary) error = %v", err)
	}
	input.ExpenseDate = tc.Trip.EndDate.AddDate(0, 0, 8)
	if _, err = svc.Create(context.Background(), actor, tc, input); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("Create(after upper boundary) error = %v", err)
	}
	input.ExpenseDate = tc.Trip.EndDate.AddDate(0, 0, 7)
	tc.Trip.Settings.MultiCurrencyEnabled = true
	input.Currency = "USD"
	if _, err = svc.Create(context.Background(), actor, tc, input); err != nil {
		t.Fatalf("Create(multi-currency enabled) error = %v", err)
	}
	tc.Trip.IsFinalized = true
	if _, err = svc.Create(context.Background(), actor, tc, input); !apperror.Is(err, "TRIP_FINALIZED") {
		t.Fatalf("Create(finalized) error = %v", err)
	}
}

func TestEveryMutationRejectsFinalizedTrip(t *testing.T) {
	for _, operation := range []string{"update", "delete", "approve", "reject"} {
		t.Run(operation, func(t *testing.T) {
			svc, repo, _, actor, tc, _ := expenseFixture(true, domaintrip.EditEveryone, domainparticipant.RolePlanner)
			repo.row = &domainexpense.Expense{ID: uuid.New(), TripID: tc.Trip.ID, ExpenseDate: tc.Trip.StartDate, Description: "Dinner", Amount: decimal.NewFromInt(10), Currency: "PHP", SplitType: domainexpense.SplitEqual, Status: domainexpense.StatusPending, CreatedByUserID: actor.UserID, Version: 1}
			tc.Trip.IsFinalized = true
			var err error
			switch operation {
			case "update":
				description := "Changed"
				_, err = svc.Update(context.Background(), actor, tc, repo.row.ID, UpdateInput{Description: &description, Version: 1})
			case "delete":
				err = svc.Delete(context.Background(), actor, tc, repo.row.ID)
			case "approve":
				_, err = svc.Approve(context.Background(), actor, tc, repo.row.ID)
			case "reject":
				_, err = svc.Reject(context.Background(), actor, tc, repo.row.ID, "duplicate")
			}
			if !apperror.Is(err, "TRIP_FINALIZED") {
				t.Fatalf("%s(finalized) error = %v", operation, err)
			}
		})
	}
}

func TestEditingApprovedExpenseResetsApproval(t *testing.T) {
	svc, repo, _, actor, tc, _ := expenseFixture(true, domaintrip.EditOwnOnly, domainparticipant.RoleParticipant)
	approver := uuid.New()
	approvedAt := time.Now().UTC()
	repo.row = &domainexpense.Expense{ID: uuid.New(), TripID: tc.Trip.ID, ExpenseDate: tc.Trip.StartDate, Description: "Dinner", Amount: decimal.NewFromInt(10), Currency: "PHP", SplitType: domainexpense.SplitEqual, Status: domainexpense.StatusApproved, CreatedByUserID: actor.UserID, ApprovedByUserID: &approver, ApprovedAt: &approvedAt, Version: 1}
	description := "Corrected dinner"
	updated, err := svc.Update(context.Background(), actor, tc, repo.row.ID, UpdateInput{Description: &description, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domainexpense.StatusPending || updated.ApprovedByUserID != nil || updated.ApprovedAt != nil {
		t.Fatalf("approval was not reset: %+v", updated)
	}
}
