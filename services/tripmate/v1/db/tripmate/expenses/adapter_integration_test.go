//go:build integration

package expenses

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	payersdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_payers"
	splitsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_splits"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func expenseIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	_ = godotenv.Load(filepath.Join(filepath.Dir(filename), "../../../../../..", ".env"))
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

func seedExpenseList(t *testing.T, db *gorm.DB, count int) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	plannerID, memberID, tripID := uuid.New(), uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err := db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?),(?,?,?,?)`, plannerID, plannerID.String()+"@example.com", "Planner", "hash", memberID, memberID.String()+"@example.com", "Member", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trips (id,code,name,base_currency,start_date,end_date,planner_id) VALUES (?,?,?,?,?,?,?)`, tripID, code, "Expense List", "PHP", time.Now().AddDate(0, 0, -10), time.Now().AddDate(0, 0, 10), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trip_participants (id,trip_id,user_id,role) VALUES (?,?,?,'planner'),(?,?,?,'participant')`, uuid.New(), tripID, plannerID, uuid.New(), tripID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	models := make([]Expense, count)
	for i := range models {
		status := string(domainexpense.StatusPending)
		var approvedBy *uuid.UUID
		var approvedAt *time.Time
		if i%2 == 0 {
			status, approvedBy = string(domainexpense.StatusApproved), &plannerID
			now := time.Now().UTC()
			approvedAt = &now
		}
		models[i] = Expense{ID: uuid.New(), TripID: tripID, ExpenseDate: time.Now().AddDate(0, 0, -(i % 5)), Description: "Dinner", Amount: decimal.NewFromInt(int64(i + 1)), Currency: "PHP", SplitType: string(domainexpense.SplitEqual), Status: status, Source: string(domainexpense.SourceManual), CreatedByUserID: plannerID, ApprovedByUserID: approvedBy, ApprovedAt: approvedAt, Version: 1}
	}
	if err := db.Create(&models).Error; err != nil {
		t.Fatal(err)
	}
	payerRows, splitRows := make([]domainexpense.Payer, count), make([]domainexpense.Split, count)
	for i, model := range models {
		payerRows[i] = domainexpense.Payer{ExpenseID: model.ID, UserID: plannerID, Amount: model.Amount}
		splitRows[i] = domainexpense.Split{ExpenseID: model.ID, UserID: memberID, Amount: model.Amount}
	}
	// The bulk API is per expense by contract.
	for i, model := range models {
		if err := payersdb.New(db).BulkCreate(context.Background(), model.ID, payerRows[i:i+1]); err != nil {
			t.Fatal(err)
		}
		if err := splitsdb.New(db).BulkCreate(context.Background(), model.ID, splitRows[i:i+1]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id IN ?", []uuid.UUID{plannerID, memberID})
	})
	return tripID, plannerID, memberID
}

func TestListByTripIDReturnsTotalsUsersAndUsesAtMostThreeQueries(t *testing.T) {
	db := expenseIntegrationDB(t)
	tripID, plannerID, memberID := seedExpenseList(t, db, 200)
	var queries, rawQueries, rowQueries atomic.Int64
	callbackName := "expenses:count_queries"
	countQuery := func(*gorm.DB) { queries.Add(1) }
	countRaw := func(*gorm.DB) { rawQueries.Add(1) }
	countRow := func(*gorm.DB) { rowQueries.Add(1) }
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, countQuery); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register(callbackName, countRaw); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Row().Before("gorm:row").Register(callbackName, countRow); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
		_ = db.Callback().Raw().Remove(callbackName)
		_ = db.Callback().Row().Remove(callbackName)
	})

	rows, total, totals, err := New(db).ListByTripID(context.Background(), tripID, expensedomain.Filter{Sort: "amount_desc", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 20 || total != 200 || !rows[0].Amount.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("list = %d/%d first=%s", len(rows), total, rows[0].Amount)
	}
	if !totals.ByCurrency["PHP"].Equal(decimal.NewFromInt(20100)) || totals.CountByStatus[domainexpense.StatusApproved] != 100 || totals.CountByStatus[domainexpense.StatusPending] != 100 {
		t.Fatalf("totals = %+v", totals)
	}
	if rows[0].CreatedBy == nil || rows[0].CreatedBy.Name != "Planner" || rows[0].Payers[0].User == nil || rows[0].Splits[0].User == nil {
		t.Fatalf("nested users are missing: %+v", rows[0])
	}
	if got := queries.Load() + rawQueries.Load() + rowQueries.Load(); got > 3 {
		t.Fatalf("queries = %d (query=%d raw=%d row=%d), want <= 3", got, queries.Load(), rawQueries.Load(), rowQueries.Load())
	}
	queries.Store(0)
	rawQueries.Store(0)
	rowQueries.Store(0)
	pending := domainexpense.StatusPending
	filtered, filteredTotal, filteredTotals, err := New(db).ListByTripID(context.Background(), tripID, expensedomain.Filter{PayerUserID: &plannerID, SplitUserID: &memberID, Status: &pending, Currency: "php", Query: "DIN", Sort: "amount_desc", Page: 2, PerPage: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 7 || filteredTotal != 100 || !filtered[0].Amount.Equal(decimal.NewFromInt(186)) {
		t.Fatalf("filtered list = %d/%d first=%s", len(filtered), filteredTotal, filtered[0].Amount)
	}
	if !filteredTotals.ByCurrency["PHP"].Equal(decimal.NewFromInt(10100)) || filteredTotals.CountByStatus[pending] != 100 {
		t.Fatalf("filtered totals = %+v", filteredTotals)
	}
	if got := queries.Load() + rawQueries.Load() + rowQueries.Load(); got > 3 {
		t.Fatalf("filtered queries = %d, want <= 3", got)
	}
}

func TestSoftDeleteHidesExpenseAndKeepsAuditRows(t *testing.T) {
	db, ctx := expenseIntegrationDB(t), context.Background()
	tripID, plannerID, memberID := seedExpenseList(t, db, 1)
	repo := New(db)
	rows, _, _, err := repo.ListByTripID(ctx, tripID, expensedomain.Filter{Page: 1, PerPage: 20})
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListByTripID() = %d, %v", len(rows), err)
	}
	id := rows[0].ID
	if err = repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if visible, total, _, listErr := repo.ListByTripID(ctx, tripID, expensedomain.Filter{Page: 1, PerPage: 20}); listErr != nil || total != 0 || len(visible) != 0 {
		t.Fatalf("deleted expense remains visible = %d/%d, %v", len(visible), total, listErr)
	}
	payers, payerErr := payersdb.New(db).ListByExpenseIDs(ctx, []uuid.UUID{id})
	splits, splitErr := splitsdb.New(db).ListByExpenseIDs(ctx, []uuid.UUID{id})
	if payerErr != nil || splitErr != nil || len(payers[id]) != 1 || payers[id][0].UserID != plannerID || len(splits[id]) != 1 || splits[id][0].UserID != memberID {
		t.Fatalf("audit rows missing: payers=%+v/%v splits=%+v/%v", payers, payerErr, splits, splitErr)
	}
}
