//go:build integration

package exchange_rates_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	ratesdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/exchange_rates"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func integrationDB(t *testing.T) *gorm.DB {
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

func TestListEffectiveTripRateShadowsGlobalAndUpsertUpdatesInPlace(t *testing.T) {
	db, ctx := integrationDB(t), context.Background()
	plannerID, tripID := uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err := db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?)`, plannerID, plannerID.String()+"@example.com", "Planner", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tripmate.trips (id,code,name,base_currency,start_date,end_date,planner_id) VALUES (?,?,?,?,?,?,?)`, tripID, code, "FX Adapter", "PHP", time.Now(), time.Now().Add(24*time.Hour), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id = ?", plannerID)
	})

	repo := ratesdb.New(db)
	created, err := repo.Upsert(ctx, domainfx.Rate{TripID: &tripID, FromCurrency: "usd", ToCurrency: "php", Rate: decimal.RequireFromString("57.25"), IsFinal: true, Source: domainfx.SourceManual})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == uuid.Nil || created.TripID == nil || !created.Rate.Equal(decimal.RequireFromString("57.25")) {
		t.Fatalf("Upsert(create) = %+v", created)
	}

	effective, err := repo.ListEffective(ctx, tripID)
	if err != nil {
		t.Fatal(err)
	}
	var usdRows int
	for _, rate := range effective {
		if rate.FromCurrency == "USD" && rate.ToCurrency == "PHP" {
			usdRows++
			if rate.TripID == nil || *rate.TripID != tripID || !rate.Rate.Equal(decimal.RequireFromString("57.25")) {
				t.Fatalf("effective USD→PHP = %+v", rate)
			}
		}
	}
	if usdRows != 1 {
		t.Fatalf("effective USD→PHP row count = %d", usdRows)
	}

	created.Rate = decimal.RequireFromString("58.125")
	updated, err := repo.Upsert(ctx, *created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || !updated.Rate.Equal(decimal.RequireFromString("58.125")) {
		t.Fatalf("Upsert(update) = %+v", updated)
	}
	global, err := repo.Get(ctx, nil, "USD", "PHP")
	if err != nil || !global.Rate.Equal(decimal.RequireFromString("56.5")) {
		t.Fatalf("global rate changed = %+v, %v", global, err)
	}
}
