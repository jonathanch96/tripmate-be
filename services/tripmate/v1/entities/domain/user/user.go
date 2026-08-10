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
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (u User) Public() PublicUser {
	return PublicUser{
		ID: u.ID, Email: u.Email, Name: u.Name, AvatarURL: u.AvatarURL,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
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
