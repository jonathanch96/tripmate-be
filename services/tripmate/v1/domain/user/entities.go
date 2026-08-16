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
