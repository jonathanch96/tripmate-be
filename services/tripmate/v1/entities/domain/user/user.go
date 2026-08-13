package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID    uuid.UUID
	Email string
	Name  string
	// PasswordHash is empty for an account created via Google sign-in.
	PasswordHash string
	// GoogleID is the Google account's stable subject ("sub") claim, set once a user signs in
	// with Google (either at account creation or by linking an existing password account).
	GoogleID  *string
	AvatarURL *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PublicUser struct {
	ID        uuid.UUID
	Email     string
	Name      string
	AvatarURL *string
	// HasAccount is false for a placeholder created by inviting an email that had no account yet
	// - they're a real trip participant (can be assigned expenses, etc.) but haven't registered
	// or signed in with Google, so they can't sign in themselves until they do.
	HasAccount bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (u User) Public() PublicUser {
	return PublicUser{
		ID: u.ID, Email: u.Email, Name: u.Name, AvatarURL: u.AvatarURL,
		HasAccount: u.PasswordHash != "" || u.GoogleID != nil,
		CreatedAt:  u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent *string
	IP        *string
	CreatedAt time.Time
}
