package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/logger"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

const dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func NewService(deps Dependencies) Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &service{deps: deps}
}

func (s *service) Register(ctx context.Context, input RegisterInput) (*Session, error) {
	email := normalizeEmail(input.Email)
	if fields := validatePassword(input.Password); len(fields) > 0 {
		return nil, apperror.WithFields("VALIDATION_FAILED", fields)
	}
	exists, err := s.deps.Repo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperror.New("EMAIL_ALREADY_REGISTERED")
	}
	passwordHash, err := s.deps.Hasher.Hash(input.Password)
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	created, err := s.deps.Repo.Create(ctx, &domainuser.User{
		ID: uuid.New(), Email: email, Name: strings.TrimSpace(input.Name), PasswordHash: passwordHash,
	})
	if err != nil {
		return nil, err
	}
	return s.issueSession(ctx, *created)
}

func (s *service) Authenticate(ctx context.Context, email, password string) (*Session, error) {
	found, err := s.deps.Repo.GetByEmail(ctx, normalizeEmail(email))
	missing := apperror.Is(err, "USER_NOT_FOUND")
	if err != nil && !missing {
		return nil, err
	}
	hash := dummyPasswordHash
	if !missing && found != nil {
		hash = found.PasswordHash
	}
	verified, verifyErr := s.deps.Hasher.Verify(password, hash)
	if verifyErr != nil && !missing {
		return nil, apperror.Wrap(verifyErr, "INTERNAL_ERROR")
	}
	if missing || found == nil || !verified {
		return nil, apperror.New("INVALID_CREDENTIALS")
	}
	return s.issueSession(ctx, *found)
}

func (s *service) Refresh(ctx context.Context, raw string) (*Session, error) {
	stored, err := s.deps.Tokens.GetByHash(ctx, tokenHash(raw))
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, apperror.New("UNAUTHENTICATED")
	}
	if stored.RevokedAt != nil {
		if err := s.deps.Tokens.RevokeAllForUser(ctx, stored.UserID); err != nil {
			return nil, err
		}
		slog.WarnContext(ctx, "refresh token reuse detected", "user_id", stored.UserID,
			"trace_id", logger.TraceID(ctx))
		return nil, apperror.New("UNAUTHENTICATED")
	}
	if !stored.ExpiresAt.After(s.deps.Clock()) {
		if err := s.deps.Tokens.Revoke(ctx, stored.ID); err != nil {
			return nil, err
		}
		return nil, apperror.New("UNAUTHENTICATED")
	}
	if err := s.deps.Tokens.Revoke(ctx, stored.ID); err != nil {
		return nil, err
	}
	entity, err := s.deps.Repo.GetByID(ctx, stored.UserID)
	if err != nil {
		return nil, err
	}
	return s.issueSession(ctx, *entity)
}

func (s *service) Logout(ctx context.Context, raw string) error {
	stored, err := s.deps.Tokens.GetByHash(ctx, tokenHash(raw))
	if err != nil {
		return err
	}
	if stored == nil || stored.RevokedAt != nil {
		return nil
	}
	return s.deps.Tokens.Revoke(ctx, stored.ID)
}

func (s *service) GetByID(ctx context.Context, id uuid.UUID) (*domainuser.User, error) {
	return s.deps.Repo.GetByID(ctx, id)
}

func (s *service) UpdateProfile(ctx context.Context, id uuid.UUID, input UpdateProfileInput) (*domainuser.User, error) {
	entity, err := s.deps.Repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		entity.Name = strings.TrimSpace(*input.Name)
	}
	if input.AvatarURL != nil {
		entity.AvatarURL = input.AvatarURL
	}
	return s.deps.Repo.Update(ctx, entity)
}

func (s *service) FindByEmail(ctx context.Context, email string) (*domainuser.User, error) {
	return s.deps.Repo.GetByEmail(ctx, normalizeEmail(email))
}

func (s *service) issueSession(ctx context.Context, entity domainuser.User) (*Session, error) {
	access, accessExpiry, err := s.deps.Issuer.IssueAccess(entity)
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	refresh, refreshExpiry, err := s.deps.Issuer.IssueRefresh(entity)
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	if err := s.deps.Tokens.Store(ctx, domainuser.RefreshToken{
		ID: uuid.New(), UserID: entity.ID, TokenHash: tokenHash(refresh), ExpiresAt: refreshExpiry,
		CreatedAt: s.deps.Clock().UTC(),
	}); err != nil {
		return nil, err
	}
	return &Session{User: entity, AccessToken: access, RefreshToken: refresh,
		AccessTokenExpiresAt: accessExpiry, RefreshTokenExpiresAt: refreshExpiry}, nil
}

func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func tokenHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validatePassword(value string) []apperror.FieldError {
	var letter, digit bool
	for _, character := range value {
		letter = letter || unicode.IsLetter(character)
		digit = digit || unicode.IsDigit(character)
	}
	fields := make([]apperror.FieldError, 0, 2)
	if len(value) < 8 {
		fields = append(fields, apperror.FieldError{Field: "password", Rule: "min", Message: "password must be at least 8 characters"})
	}
	if !letter || !digit {
		fields = append(fields, apperror.FieldError{Field: "password", Rule: "complexity", Message: "password must contain a letter and a digit"})
	}
	return fields
}
