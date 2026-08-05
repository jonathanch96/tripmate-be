//go:build integration

package users

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/joho/godotenv"
)

func TestUsersAdapterAgainstPostgres(t *testing.T) {
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
	repo := NewGormPostgresqlAdapter(db)
	id := uuid.New()
	t.Cleanup(func() { db.Exec("DELETE FROM tripmate.users WHERE id = ?", id) })
	entity := &domainuser.User{ID: id, Email: "integration-" + id.String() + "@example.com", Name: "Integration", PasswordHash: "hash"}
	created, err := repo.Create(context.Background(), entity)
	if err != nil || created.ID != id {
		t.Fatalf("create = %+v, %v", created, err)
	}
	byID, err := repo.GetByID(context.Background(), id)
	if err != nil || byID.Email != entity.Email {
		t.Fatalf("get by id = %+v, %v", byID, err)
	}
	byEmail, err := repo.GetByEmail(context.Background(), entity.Email)
	if err != nil || byEmail.ID != id {
		t.Fatalf("get by email = %+v, %v", byEmail, err)
	}
	duplicate := *entity
	duplicate.ID = uuid.New()
	if _, err := repo.Create(context.Background(), &duplicate); !apperror.Is(err, "EMAIL_ALREADY_REGISTERED") {
		t.Fatalf("duplicate = %v", err)
	}
	if err := db.Exec("UPDATE tripmate.users SET deleted_at = now() WHERE id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByEmail(context.Background(), entity.Email); !apperror.Is(err, "USER_NOT_FOUND") {
		t.Fatalf("soft deleted lookup = %v", err)
	}
}
