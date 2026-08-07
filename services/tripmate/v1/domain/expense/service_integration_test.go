//go:build integration

package expense_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	payersdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_payers"
	splitsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_splits"
	expensesdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expenses"
	outboxdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/outbox_events"
	participantsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/trip_participants"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type failingSplits struct {
	cause error
	seen  *uuid.UUID
}

func (f failingSplits) BulkCreate(_ context.Context, expenseID uuid.UUID, _ []domainexpense.Split) error {
	*f.seen = expenseID
	return f.cause
}
func (f failingSplits) ListByExpenseIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]domainexpense.Split, error) {
	return nil, nil
}
func (f failingSplits) DeleteByExpenseID(context.Context, uuid.UUID) error { return nil }

type failingOutbox struct {
	cause error
	seen  *uuid.UUID
}

func (f failingOutbox) Create(_ context.Context, row *event.OutboxEvent) error {
	*f.seen = row.AggregateID
	return f.cause
}

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	_ = godotenv.Load(filepath.Join(filepath.Dir(filename), "../../../../..", ".env"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := config.NewDatabase(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCreateRollsBackExpensePayersAndOutboxWhenSplitWriteFails(t *testing.T) {
	db, ctx := integrationDB(t), context.Background()
	plannerID, memberID, tripID := uuid.New(), uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err := db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?),(?,?,?,?)`, plannerID, plannerID.String()+"@example.com", "Planner", "hash", memberID, memberID.String()+"@example.com", "Member", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trips (id,code,name,base_currency,start_date,end_date,planner_id,setting_edit_permission) VALUES (?,?,?,?,?,?,?,'everyone')`, tripID, code, "Atomic Expense", "PHP", time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trip_participants (id,trip_id,user_id,role) VALUES (?,?,?,'planner'),(?,?,?,'participant')`, uuid.New(), tripID, plannerID, uuid.New(), tripID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id IN ?", []uuid.UUID{plannerID, memberID})
	})

	expenseIDSeen := uuid.Nil
	expenseRepo := expensesdb.New(db)
	payerRepo := payersdb.New(db)
	cause := errors.New("injected split failure")
	service := expensedomain.NewService(expensedomain.Dependencies{
		Expenses: expenseRepo, Payers: payerRepo, Splits: failingSplits{cause: cause, seen: &expenseIDSeen},
		Participants: participantsdb.New(db), Outbox: outboxdb.New(db), UOW: appdb.NewGormUnitOfWork(db),
	})
	tc := tripctx.TripContext{Trip: domaintrip.Trip{ID: tripID, Code: code, BaseCurrency: "PHP", StartDate: time.Now().Add(-24 * time.Hour), EndDate: time.Now().Add(24 * time.Hour), Settings: domaintrip.Settings{EditPermission: domaintrip.EditEveryone}}, Participant: domainparticipant.Participant{TripID: tripID, UserID: plannerID, Role: domainparticipant.RolePlanner}}
	created, err := service.Create(ctx, identity.Identity{UserID: plannerID}, tc, expensedomain.CreateInput{ExpenseDate: time.Now(), Description: "Dinner", Amount: decimal.NewFromInt(12000), Currency: "PHP", SplitType: domainexpense.SplitEqual, Payers: []domainexpense.Payer{{UserID: plannerID, Amount: decimal.NewFromInt(8000)}, {UserID: memberID, Amount: decimal.NewFromInt(4000)}}, Participants: []uuid.UUID{plannerID, memberID}})
	if created != nil || !errors.Is(err, cause) {
		t.Fatalf("Create() = %+v, %v", created, err)
	}
	// The service owns the generated ID, so verify rollback through the public
	// list and payer repository interfaces rather than inspecting internals.
	rows, total, _, listErr := expenseRepo.ListByTripID(ctx, tripID, expensedomain.Filter{Page: 1, PerPage: 20})
	if listErr != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("rolled-back expenses = %d/%d, %v", len(rows), total, listErr)
	}
	if expenseIDSeen == uuid.Nil {
		t.Fatal("split boundary did not receive the generated expense ID")
	}
	if payers, payerErr := payerRepo.ListByExpenseIDs(ctx, []uuid.UUID{expenseIDSeen}); payerErr != nil || len(payers[expenseIDSeen]) != 0 {
		t.Fatalf("rolled-back payers = %+v, %v", payers, payerErr)
	}
}

func TestOutboxFailureRollsBackWholeExpenseAndSuccessfulCreateWritesOneEvent(t *testing.T) {
	db, ctx := integrationDB(t), context.Background()
	plannerID, memberID, memberTwoID, memberThreeID, tripID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err := db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?),(?,?,?,?),(?,?,?,?),(?,?,?,?)`, plannerID, plannerID.String()+"@example.com", "Planner", "hash", memberID, memberID.String()+"@example.com", "Member", "hash", memberTwoID, memberTwoID.String()+"@example.com", "Member Two", "hash", memberThreeID, memberThreeID.String()+"@example.com", "Member Three", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trips (id,code,name,base_currency,start_date,end_date,planner_id,setting_edit_permission) VALUES (?,?,?,?,?,?,?,'everyone')`, tripID, code, "Outbox Expense", "PHP", time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trip_participants (id,trip_id,user_id,role) VALUES (?,?,?,'planner'),(?,?,?,'participant'),(?,?,?,'participant'),(?,?,?,'participant')`, uuid.New(), tripID, plannerID, uuid.New(), tripID, memberID, uuid.New(), tripID, memberTwoID, uuid.New(), tripID, memberThreeID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id IN ?", []uuid.UUID{plannerID, memberID, memberTwoID, memberThreeID})
	})
	tc := tripctx.TripContext{Trip: domaintrip.Trip{ID: tripID, Code: code, BaseCurrency: "PHP", StartDate: time.Now().Add(-24 * time.Hour), EndDate: time.Now().Add(24 * time.Hour), Settings: domaintrip.Settings{EditPermission: domaintrip.EditEveryone}}, Participant: domainparticipant.Participant{TripID: tripID, UserID: plannerID, Role: domainparticipant.RolePlanner}}
	input := expensedomain.CreateInput{ExpenseDate: time.Now(), Description: "Dinner", Amount: decimal.NewFromInt(12000), Currency: "PHP", SplitType: domainexpense.SplitEqual, Payers: []domainexpense.Payer{{UserID: plannerID, Amount: decimal.NewFromInt(8000)}, {UserID: memberID, Amount: decimal.NewFromInt(4000)}}, Participants: []uuid.UUID{plannerID, memberID, memberTwoID, memberThreeID}}
	expenseRepo, payerRepo, splitRepo := expensesdb.New(db), payersdb.New(db), splitsdb.New(db)
	cause, failedID := errors.New("injected outbox failure"), uuid.Nil
	failingService := expensedomain.NewService(expensedomain.Dependencies{Expenses: expenseRepo, Payers: payerRepo, Splits: splitRepo, Participants: participantsdb.New(db), Outbox: failingOutbox{cause: cause, seen: &failedID}, UOW: appdb.NewGormUnitOfWork(db)})
	if created, err := failingService.Create(ctx, identity.Identity{UserID: plannerID}, tc, input); created != nil || !errors.Is(err, cause) {
		t.Fatalf("Create(outbox failure) = %+v, %v", created, err)
	}
	if failedID == uuid.Nil {
		t.Fatal("outbox did not observe the generated expense")
	}
	if rows, total, _, err := expenseRepo.ListByTripID(ctx, tripID, expensedomain.Filter{Page: 1, PerPage: 20}); err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("expense survived outbox failure = %d/%d, %v", len(rows), total, err)
	}
	if payers, err := payerRepo.ListByExpenseIDs(ctx, []uuid.UUID{failedID}); err != nil || len(payers[failedID]) != 0 {
		t.Fatalf("payers survived outbox failure = %+v, %v", payers, err)
	}
	if splits, err := splitRepo.ListByExpenseIDs(ctx, []uuid.UUID{failedID}); err != nil || len(splits[failedID]) != 0 {
		t.Fatalf("splits survived outbox failure = %+v, %v", splits, err)
	}

	outboxRepo := outboxdb.New(db)
	service := expensedomain.NewService(expensedomain.Dependencies{Expenses: expenseRepo, Payers: payerRepo, Splits: splitRepo, Participants: participantsdb.New(db), Outbox: outboxRepo, UOW: appdb.NewGormUnitOfWork(db)})
	created, err := service.Create(ctx, identity.Identity{UserID: plannerID}, tc, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Payers) != 2 || len(created.Splits) != 4 {
		t.Fatalf("created shape = 1 expense/%d payer rows/%d split rows", len(created.Payers), len(created.Splits))
	}
	count, err := outboxRepo.CountByAggregateID(ctx, created.ID)
	if err != nil || count != 1 {
		t.Fatalf("outbox event count = %d, %v", count, err)
	}
	var invalidRows int64
	err = db.Raw(`
		SELECT count(*)
		FROM tripmate.expenses AS e
		JOIN (SELECT expense_id, sum(amount) AS total FROM tripmate.expense_payers GROUP BY expense_id) AS p ON p.expense_id = e.id
		JOIN (SELECT expense_id, sum(amount) AS total FROM tripmate.expense_splits GROUP BY expense_id) AS s ON s.expense_id = e.id
		WHERE e.trip_id = ? AND (p.total <> e.amount OR s.total <> e.amount)`, tripID).Scan(&invalidRows).Error
	if err != nil || invalidRows != 0 {
		t.Fatalf("expense monetary invariant violations = %d, %v", invalidRows, err)
	}
}

func TestOutboxFailureRollsBackUpdateDeleteApproveAndReject(t *testing.T) {
	db, ctx := integrationDB(t), context.Background()
	plannerID, memberID, tripID := uuid.New(), uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err := db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?),(?,?,?,?)`, plannerID, plannerID.String()+"@example.com", "Planner", "hash", memberID, memberID.String()+"@example.com", "Member", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trips (id,code,name,base_currency,start_date,end_date,planner_id,setting_edit_permission,setting_approval_expenses) VALUES (?,?,?,?,?,?,?,'everyone',true)`, tripID, code, "Mutation Rollback", "PHP", time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trip_participants (id,trip_id,user_id,role) VALUES (?,?,?,'planner'),(?,?,?,'participant')`, uuid.New(), tripID, plannerID, uuid.New(), tripID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id IN ?", []uuid.UUID{plannerID, memberID})
	})

	tc := tripctx.TripContext{Trip: domaintrip.Trip{ID: tripID, Code: code, BaseCurrency: "PHP", StartDate: time.Now().Add(-24 * time.Hour), EndDate: time.Now().Add(24 * time.Hour), Settings: domaintrip.Settings{EditPermission: domaintrip.EditEveryone, ApprovalRequiredExpenses: true}}, Participant: domainparticipant.Participant{TripID: tripID, UserID: plannerID, Role: domainparticipant.RolePlanner}}
	expenseRepo, payerRepo, splitRepo, outboxRepo := expensesdb.New(db), payersdb.New(db), splitsdb.New(db), outboxdb.New(db)
	participants := participantsdb.New(db)
	uow := appdb.NewGormUnitOfWork(db)
	goodService := expensedomain.NewService(expensedomain.Dependencies{Expenses: expenseRepo, Payers: payerRepo, Splits: splitRepo, Participants: participants, Outbox: outboxRepo, UOW: uow})
	created, err := goodService.Create(ctx, identity.Identity{UserID: plannerID}, tc, expensedomain.CreateInput{ExpenseDate: time.Now(), Description: "Original", Amount: decimal.NewFromInt(100), Currency: "PHP", SplitType: domainexpense.SplitEqual, Payers: []domainexpense.Payer{{UserID: plannerID, Amount: decimal.NewFromInt(100)}}, Participants: []uuid.UUID{plannerID, memberID}})
	if err != nil {
		t.Fatal(err)
	}

	for _, operation := range []string{"update", "delete", "approve", "reject"} {
		t.Run(operation, func(t *testing.T) {
			cause, seen := errors.New("injected "+operation+" outbox failure"), uuid.Nil
			failingService := expensedomain.NewService(expensedomain.Dependencies{Expenses: expenseRepo, Payers: payerRepo, Splits: splitRepo, Participants: participants, Outbox: failingOutbox{cause: cause, seen: &seen}, UOW: uow})
			var mutationErr error
			switch operation {
			case "update":
				amount := decimal.NewFromInt(120)
				_, mutationErr = failingService.Update(ctx, identity.Identity{UserID: plannerID}, tc, created.ID, expensedomain.UpdateInput{Amount: &amount, Payers: []domainexpense.Payer{{UserID: plannerID, Amount: amount}}, Participants: []uuid.UUID{plannerID, memberID}, Version: created.Version})
			case "delete":
				mutationErr = failingService.Delete(ctx, identity.Identity{UserID: plannerID}, tc, created.ID)
			case "approve":
				_, mutationErr = failingService.Approve(ctx, identity.Identity{UserID: plannerID}, tc, created.ID)
			case "reject":
				_, mutationErr = failingService.Reject(ctx, identity.Identity{UserID: plannerID}, tc, created.ID, "duplicate")
			}
			if !errors.Is(mutationErr, cause) || seen != created.ID {
				t.Fatalf("%s() error/aggregate = %v/%s", operation, mutationErr, seen)
			}
			persisted, getErr := expenseRepo.GetByID(ctx, created.ID)
			if getErr != nil || persisted.Description != "Original" || !persisted.Amount.Equal(decimal.NewFromInt(100)) || persisted.Status != domainexpense.StatusPending || persisted.Version != created.Version {
				t.Fatalf("%s rollback left expense = %+v, %v", operation, persisted, getErr)
			}
			count, countErr := outboxRepo.CountByAggregateID(ctx, created.ID)
			if countErr != nil || count != 1 {
				t.Fatalf("%s rollback outbox count = %d, %v", operation, count, countErr)
			}
		})
	}
}
