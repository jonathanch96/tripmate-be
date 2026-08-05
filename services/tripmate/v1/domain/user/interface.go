package user

import (
	"context"
	"time"

	"github.com/google/uuid"
	appjwt "github.com/jblabs/tripmate-be/pkg/jwt"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type Service interface {
	Register(context.Context, RegisterInput) (*Session, error)
	Authenticate(context.Context, string, string) (*Session, error)
	Refresh(context.Context, string) (*Session, error)
	Logout(context.Context, string) error
	GetByID(context.Context, uuid.UUID) (*domainuser.User, error)
	UpdateProfile(context.Context, uuid.UUID, UpdateProfileInput) (*domainuser.User, error)
	FindByEmail(context.Context, string) (*domainuser.User, error)
}

type Repository interface {
	Create(context.Context, *domainuser.User) (*domainuser.User, error)
	GetByID(context.Context, uuid.UUID) (*domainuser.User, error)
	GetByEmail(context.Context, string) (*domainuser.User, error)
	ExistsByEmail(context.Context, string) (bool, error)
	Update(context.Context, *domainuser.User) (*domainuser.User, error)
	ListByIDs(context.Context, []uuid.UUID) ([]domainuser.User, error)
}

type TokenRepository interface {
	Store(context.Context, domainuser.RefreshToken) error
	GetByHash(context.Context, string) (*domainuser.RefreshToken, error)
	Revoke(context.Context, uuid.UUID) error
	RevokeAllForUser(context.Context, uuid.UUID) error
}

type Hasher interface {
	Hash(string) (string, error)
	Verify(string, string) (bool, error)
}

type TokenIssuer interface {
	IssueAccess(domainuser.User) (string, time.Time, error)
	IssueRefresh(domainuser.User) (string, time.Time, error)
	ParseAccess(string) (*appjwt.Claims, error)
}
