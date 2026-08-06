//go:build integration

package trip_invitations

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
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	"github.com/joho/godotenv"
)

func TestInvitationAdapterLifecycle(t *testing.T) {
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

	ctx := context.Background()
	plannerID, tripID := uuid.New(), uuid.New()
	code := uuid.NewString()[:6]
	if err = db.Exec("INSERT INTO tripmate.users (id,email,name,password_hash) VALUES (?,?,?,?)", plannerID, plannerID.String()+"@example.com", "Planner", "hash").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO tripmate.trips
		(id,code,name,base_currency,start_date,end_date,planner_id)
		VALUES (?,?,?,?,?,?,?)`, tripID, code, "Invitation Adapter", "USD", time.Now(), time.Now().Add(24*time.Hour), plannerID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tripmate.trips WHERE id = ?", tripID)
		db.Exec("DELETE FROM tripmate.users WHERE id = ?", plannerID)
	})

	repo := NewGormPostgresqlAdapter(db)
	now := time.Now().UTC()
	pending, err := repo.Create(ctx, &domaininvitation.Invitation{
		ID: uuid.New(), TripID: tripID, Email: "friend@example.com", Token: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status: domaininvitation.StatusPending, InvitedBy: plannerID, ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, findErr := repo.GetByToken(ctx, pending.Token); findErr != nil || found.ID != pending.ID {
		t.Fatalf("GetByToken() = %+v, %v", found, findErr)
	}
	if found, findErr := repo.GetPending(ctx, tripID, "friend@example.com"); findErr != nil || found.ID != pending.ID {
		t.Fatalf("GetPending() = %+v, %v", found, findErr)
	}
	if _, err = repo.Create(ctx, &domaininvitation.Invitation{
		ID: uuid.New(), TripID: tripID, Email: "expired@example.com", Token: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Status: domaininvitation.StatusPending, InvitedBy: plannerID, ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListPendingByEmail(ctx, "friend@example.com")
	if err != nil || len(rows) != 1 || rows[0].ID != pending.ID {
		t.Fatalf("ListPendingByEmail() = %+v, %v", rows, err)
	}

	acceptedAt := now
	pending.Status = domaininvitation.StatusAccepted
	pending.AcceptedAt = &acceptedAt
	updated, err := repo.Update(ctx, pending)
	if err != nil || updated.Status != domaininvitation.StatusAccepted || updated.AcceptedAt == nil {
		t.Fatalf("Update() = %+v, %v", updated, err)
	}

	revocable, err := repo.Create(ctx, &domaininvitation.Invitation{
		ID: uuid.New(), TripID: tripID, Email: "revoke@example.com", Token: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Status: domaininvitation.StatusPending, InvitedBy: plannerID, ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Revoke(ctx, revocable.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err = repo.Revoke(ctx, revocable.ID); !apperror.Is(err, "INVITATION_NOT_FOUND") {
		t.Fatalf("second Revoke() error = %v", err)
	}
	all, err := repo.ListByTrip(ctx, tripID)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListByTrip() = %d rows, %v", len(all), err)
	}
}
