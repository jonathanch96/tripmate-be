//go:build integration

package trip_participants

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
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func TestListByTripIDUsesOneQuery(t *testing.T) {
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

	plannerID, memberID, tripID := uuid.New(), uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err = db.Exec(`INSERT INTO tripmate.users (id,email,name,password_hash) VALUES
		(?,?,?,?),(?,?,?,?)`, plannerID, plannerID.String()+"@example.com", "Planner", "hash", memberID, memberID.String()+"@example.com", "Member", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO tripmate.trips
		(id,code,name,base_currency,start_date,end_date,planner_id)
		VALUES (?,?,?,?,?,?,?)`, tripID, code, "Query Count", "USD", time.Now(), time.Now().Add(24*time.Hour), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO tripmate.trip_participants (id,trip_id,user_id,role)
		VALUES (?,?,?,'planner'),(?,?,?,'participant')`, uuid.New(), tripID, plannerID, uuid.New(), tripID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id IN ?", []uuid.UUID{plannerID, memberID})
	})

	var queries atomic.Int64
	callbackName := "trip_participants:count_queries"
	count := func(*gorm.DB) {
		queries.Add(1)
	}
	if err = db.Callback().Query().Before("gorm:query").Register(callbackName, count); err != nil {
		t.Fatal(err)
	}
	if err = db.Callback().Raw().Before("gorm:raw").Register(callbackName, count); err != nil {
		t.Fatal(err)
	}
	if err = db.Callback().Row().Before("gorm:row").Register(callbackName, count); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
		_ = db.Callback().Raw().Remove(callbackName)
		_ = db.Callback().Row().Remove(callbackName)
	})

	rows, err := New(db).ListByTripID(context.Background(), tripID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("participants = %d, want 2", len(rows))
	}
	if got := queries.Load(); got != 1 {
		t.Fatalf("queries = %d, want 1 (participant list must not be N+1)", got)
	}
}
