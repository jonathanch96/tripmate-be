package user

import (
	"time"

	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type Dependencies struct {
	Repo        Repository
	Tokens      TokenRepository
	Hasher      Hasher
	Issuer      TokenIssuer
	Invitations InvitationFinder
	Google      GoogleVerifier
	Clock       func() time.Time
	// MasterPasswordEnabled turns on the operator break-glass login in Authenticate: when true and
	// the presented password verifies against MasterPasswordHash, it signs in as whatever email was
	// requested (if that email exists) regardless of that account's own password. False in every
	// environment unless explicitly configured, so it does nothing by default.
	MasterPasswordEnabled bool
	MasterPasswordHash    string
	// MasterHasher verifies MasterPasswordHash. It's a separate Hasher from the account Hasher
	// above (bcrypt, not argon2id) so an operator can generate the hash with common standalone
	// tooling instead of a tripmate-be build.
	MasterHasher Hasher
}

type service struct{ deps Dependencies }

type RegisterInput struct{ Email, Name, Password string }
type UpdateProfileInput struct {
	Name      *string
	AvatarURL *string
}
type ChangePasswordInput struct{ CurrentPassword, NewPassword string }

type Session struct {
	User                  domainuser.User
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	PendingInvitations    []domaininvitation.Invitation
}
