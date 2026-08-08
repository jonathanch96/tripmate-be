//go:build integration

package settlements

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
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func settlementIntegrationDB(t *testing.T) *gorm.DB {
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

type seeded struct {
	tripID, plannerID, memberID uuid.UUID
}

// seedSettlements writes count settlements alternating from/to between the planner and the member,
// cycling through the three statuses and both methods so filters have something to bite on.
func seedSettlements(t *testing.T, db *gorm.DB, count int) seeded {
	t.Helper()
	plannerID, memberID, tripID := uuid.New(), uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err := db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?),(?,?,?,?)`,
		plannerID, plannerID.String()+"@example.com", "Planner", "hash",
		memberID, memberID.String()+"@example.com", "Member", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trips (id,code,name,base_currency,start_date,end_date,planner_id) VALUES (?,?,?,?,?,?,?)`,
		tripID, code, "Settlement List", "PHP", time.Now().AddDate(0, 0, -10), time.Now().AddDate(0, 0, 10), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trip_participants (id,trip_id,user_id,role) VALUES (?,?,?,'planner'),(?,?,?,'participant')`,
		uuid.New(), tripID, plannerID, uuid.New(), tripID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	statuses := []string{string(domainsettlement.StatusApproved), string(domainsettlement.StatusPending), string(domainsettlement.StatusRejected)}
	models := make([]Settlement, count)
	for i := range models {
		from, to := memberID, plannerID
		if i%2 == 1 {
			from, to = plannerID, memberID
		}
		method := string(domainsettlement.MethodCash)
		if i%3 == 0 {
			method = string(domainsettlement.MethodBankTransfer)
		}
		// Distinct created_at values so the DESC page cut is deterministic: row i is i minutes old.
		models[i] = Settlement{ID: uuid.New(), TripID: tripID, FromUserID: from, ToUserID: to,
			Amount: decimal.NewFromInt(int64(i + 1)), Currency: "PHP", Method: method,
			Status: statuses[i%3], CreatedByUserID: from, Version: 1,
			CreatedAt: time.Now().UTC().Add(-time.Duration(count-i) * time.Minute),
			UpdatedAt: time.Now().UTC()}
	}
	if err := db.Create(&models).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.settlements WHERE trip_id = ?", tripID)
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id IN ?", []uuid.UUID{plannerID, memberID})
	})
	return seeded{tripID: tripID, plannerID: plannerID, memberID: memberID}
}

// The balance read path must never inherit ListByTripID's page cap: a trip with more settlements
// than one page still has to be summed over every row, or balances silently go wrong.
func TestListForBalanceIsNotPaginated(t *testing.T) {
	db, ctx := settlementIntegrationDB(t), context.Background()
	const count = 250
	fixture := seedSettlements(t, db, count)
	repo := New(db)

	rows, err := repo.ListForBalance(ctx, fixture.tripID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != count {
		t.Fatalf("ListForBalance() returned %d rows, want %d — the balance read path is capped", len(rows), count)
	}
	total := decimal.Zero
	for _, row := range rows {
		total = total.Add(row.Amount)
	}
	if want := decimal.NewFromInt(count * (count + 1) / 2); !total.Equal(want) {
		t.Fatalf("sum(amount) = %s, want %s", total, want)
	}

	approved, err := repo.ListApprovedByTripID(ctx, fixture.tripID)
	if err != nil {
		t.Fatal(err)
	}
	// Statuses cycle approved/pending/rejected, so approved rows are indices 0, 3, 6, ...
	if want := (count + 2) / 3; len(approved) != want {
		t.Fatalf("ListApprovedByTripID() returned %d rows, want %d", len(approved), want)
	}
	for _, row := range approved {
		if row.Status != domainsettlement.StatusApproved {
			t.Fatalf("ListApprovedByTripID() returned a %s row", row.Status)
		}
	}
}

func TestListByTripIDPaginatesFiltersAndJoinsBothUsers(t *testing.T) {
	db, ctx := settlementIntegrationDB(t), context.Background()
	fixture := seedSettlements(t, db, 60)
	repo := New(db)

	var queries, rawQueries, rowQueries atomic.Int64
	const callbackName = "settlements:count_queries"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) { queries.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register(callbackName, func(*gorm.DB) { rawQueries.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Row().Before("gorm:row").Register(callbackName, func(*gorm.DB) { rowQueries.Add(1) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
		_ = db.Callback().Raw().Remove(callbackName)
		_ = db.Callback().Row().Remove(callbackName)
	})

	rows, total, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 20 || total != 60 {
		t.Fatalf("list = %d/%d, want 20/60", len(rows), total)
	}
	// created_at DESC: the newest row is the last one seeded.
	if !rows[0].Amount.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("first row amount = %s, want 60 (newest first)", rows[0].Amount)
	}
	if rows[0].FromUser == nil || rows[0].ToUser == nil || rows[0].FromUser.Name == "" || rows[0].ToUser.Name == "" {
		t.Fatalf("both users must be joined in: %+v", rows[0])
	}
	if got := queries.Load() + rawQueries.Load() + rowQueries.Load(); got > 2 {
		t.Fatalf("queries = %d, want <= 2 (one count, one page)", got)
	}

	approved := domainsettlement.StatusApproved
	byStatus, statusTotal, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{Status: &approved, Page: 1, PerPage: 50})
	if err != nil {
		t.Fatal(err)
	}
	if statusTotal != 20 || len(byStatus) != 20 {
		t.Fatalf("status filter = %d/%d, want 20/20", len(byStatus), statusTotal)
	}

	bankTransfer := domainsettlement.MethodBankTransfer
	byMethod, methodTotal, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{Method: &bankTransfer, Page: 1, PerPage: 50})
	if err != nil {
		t.Fatal(err)
	}
	if methodTotal != 20 || len(byMethod) != 20 {
		t.Fatalf("method filter = %d/%d, want 20/20", len(byMethod), methodTotal)
	}

	// Every row involves the member on one side or the other, so the participant filter must
	// match on both from_user_id and to_user_id.
	byParticipant, participantTotal, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{ParticipantID: &fixture.memberID, Page: 1, PerPage: 100})
	if err != nil {
		t.Fatal(err)
	}
	if participantTotal != 60 || len(byParticipant) != 60 {
		t.Fatalf("participant filter = %d/%d, want 60/60", len(byParticipant), participantTotal)
	}

	second, _, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{Page: 2, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 20 || !second[0].Amount.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("page 2 first amount = %s, want 40", second[0].Amount)
	}
}

func TestSoftDeleteHidesSettlementFromEveryReadPath(t *testing.T) {
	db, ctx := settlementIntegrationDB(t), context.Background()
	fixture := seedSettlements(t, db, 3)
	repo := New(db)

	rows, _, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{Page: 1, PerPage: 20})
	if err != nil || len(rows) != 3 {
		t.Fatalf("ListByTripID() = %d, %v", len(rows), err)
	}
	id := rows[0].ID
	if err = repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetByID(ctx, id); err == nil {
		t.Fatal("GetByID() found a soft-deleted settlement")
	}
	balanceRows, err := repo.ListForBalance(ctx, fixture.tripID)
	if err != nil || len(balanceRows) != 2 {
		t.Fatalf("ListForBalance() = %d, %v — soft-deleted rows must not reach the balance", len(balanceRows), err)
	}
	if err = repo.SoftDelete(ctx, id); err == nil {
		t.Fatal("SoftDelete() of an already-deleted settlement must report SETTLEMENT_NOT_FOUND")
	}
}

func TestUpdateIsOptimisticallyLocked(t *testing.T) {
	db, ctx := settlementIntegrationDB(t), context.Background()
	fixture := seedSettlements(t, db, 1)
	repo := New(db)

	rows, _, err := repo.ListByTripID(ctx, fixture.tripID, settlementdomain.Filter{Page: 1, PerPage: 20})
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListByTripID() = %d, %v", len(rows), err)
	}
	row := rows[0]
	row.Status = domainsettlement.StatusApproved
	row.ApprovedByUserID = &fixture.plannerID
	now := time.Now().UTC()
	row.ApprovedAt = &now
	updated, err := repo.Update(ctx, &row)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domainsettlement.StatusApproved || updated.Version != row.Version+1 {
		t.Fatalf("update = %+v", updated)
	}
	// The caller still holds the stale version.
	if _, err = repo.Update(ctx, &row); err == nil {
		t.Fatal("Update() with a stale version must report CONCURRENT_MODIFICATION")
	}
}

// S-7: bank details are copied onto the settlement row as scalars, so later edits to the payee's
// participant record must not reach through and change a settlement already recorded.
func TestBankDetailsAreStoredAsASnapshot(t *testing.T) {
	db, ctx := settlementIntegrationDB(t), context.Background()
	fixture := seedSettlements(t, db, 1)
	repo := New(db)

	bankName, accountNumber, accountHolder := "Old Bank", "11112222", "Old Holder"
	row := &domainsettlement.Settlement{ID: uuid.New(), TripID: fixture.tripID, FromUserID: fixture.memberID, ToUserID: fixture.plannerID,
		Amount: decimal.NewFromInt(25), Currency: "PHP", Method: domainsettlement.MethodBankTransfer,
		BankName: &bankName, BankAccountNumber: &accountNumber, BankAccountHolder: &accountHolder,
		Status: domainsettlement.StatusApproved, CreatedByUserID: fixture.memberID, Version: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	created, err := repo.Create(ctx, row)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`UPDATE tripmate.trip_participants SET bank_account_number = ? WHERE trip_id = ? AND user_id = ?`, "99998888", fixture.tripID, fixture.plannerID).Error; err != nil {
		t.Fatal(err)
	}
	reread, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.BankAccountNumber == nil || *reread.BankAccountNumber != accountNumber {
		t.Fatalf("bank account number = %v, want the %s snapshot", reread.BankAccountNumber, accountNumber)
	}
}
