package user

import (
	"time"

	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type Dependencies struct {
	Repo   Repository
	Tokens TokenRepository
	Hasher Hasher
	Issuer TokenIssuer
	Clock  func() time.Time
}

type service struct{ deps Dependencies }

type RegisterInput struct{ Email, Name, Password string }
type UpdateProfileInput struct {
	Name      *string
	AvatarURL *string
}

type Session struct {
	User                  domainuser.User
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}
