//go:build integration

package refresh_tokens

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
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/joho/godotenv"
)

func TestRefreshTokenAdapterAgainstPostgres(t *testing.T) {
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
	userID, tokenID := uuid.New(), uuid.New()
	if err := db.Exec("INSERT INTO tripmate.users (id, email, name, password_hash) VALUES (?, ?, ?, ?)", userID, "token-"+userID.String()+"@example.com", "Token", "hash").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM tripmate.users WHERE id = ?", userID) })
	repo := NewGormPostgresqlAdapter(db)
	if missing, err := repo.GetByHash(context.Background(), "missing"); err != nil || missing != nil {
		t.Fatalf("missing = %+v, %v", missing, err)
	}
	entity := domainuser.RefreshToken{ID: tokenID, UserID: userID, TokenHash: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.Store(context.Background(), entity); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetByHash(context.Background(), entity.TokenHash)
	if err != nil || stored == nil || stored.RevokedAt != nil {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	if err := repo.Revoke(context.Background(), tokenID); err != nil {
		t.Fatal(err)
	}
	stored, _ = repo.GetByHash(context.Background(), entity.TokenHash)
	if stored.RevokedAt == nil {
		t.Fatal("token was not revoked")
	}
}
