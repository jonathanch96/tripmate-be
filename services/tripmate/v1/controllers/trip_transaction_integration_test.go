//go:build integration

package controllers

import (
	"context"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	tripsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/trips"
	dp "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	dt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/joho/godotenv"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTripCreationIsAtomicAndSettingsUseOptimisticLock(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	_ = godotenv.Load(filepath.Join(filepath.Dir(file), "../../../..", ".env"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := config.NewDatabase(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	planner := uuid.New()
	email := "s2-" + planner.String() + "@example.com"
	if err = db.Exec("INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?)", planner, email, "S2", "hash").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE planner_id = ?", planner)
		db.Exec("DELETE FROM tripmate.users WHERE id = ?", planner)
	})
	failed := &dt.Trip{ID: uuid.New(), Code: "FAIL01", Name: "Fail", BaseCurrency: "USD", StartDate: time.Now(), EndDate: time.Now(), PlannerID: planner, Settings: dt.Settings{EditPermission: dt.EditEveryone}, Version: 1}
	if _, err = (tripTransactor{db}).CreateTripWithPlanner(ctx, failed, &dp.Participant{ID: uuid.New(), TripID: failed.ID, UserID: uuid.New(), Role: dp.RolePlanner}); err == nil {
		t.Fatal("fault injection should fail")
	}
	var count int64
	db.Table("tripmate.trips").Where("id = ?", failed.ID).Count(&count)
	if count != 0 {
		t.Fatal("trip survived participant insert failure")
	}
	trip := &dt.Trip{ID: uuid.New(), Code: "LOCK01", Name: "Lock", BaseCurrency: "USD", StartDate: time.Now(), EndDate: time.Now(), PlannerID: planner, Settings: dt.Settings{EditPermission: dt.EditEveryone}, Version: 1}
	created, err := (tripTransactor{db}).CreateTripWithPlanner(ctx, trip, &dp.Participant{ID: uuid.New(), TripID: trip.ID, UserID: planner, Role: dp.RolePlanner})
	if err != nil {
		t.Fatal(err)
	}
	repo := tripsdb.New(db)
	created.Name = "Winner"
	if _, err = repo.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	created.Name = "Stale"
	if _, err = repo.Update(ctx, created); !apperror.Is(err, "CONCURRENT_MODIFICATION") {
		t.Fatalf("stale update=%v", err)
	}
}
