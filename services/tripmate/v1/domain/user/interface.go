package user

import (
	"context"
	"time"

	"github.com/google/uuid"
	appjwt "github.com/jblabs/tripmate-be/pkg/jwt"
	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type Service interface {
	Register(context.Context, RegisterInput) (*Session, error)
	Authenticate(context.Context, string, string) (*Session, error)
	AuthenticateGoogle(context.Context, string) (*Session, error)
	Refresh(context.Context, string) (*Session, error)
	Logout(context.Context, string) error
	GetByID(context.Context, uuid.UUID) (*domainuser.User, error)
	UpdateProfile(context.Context, uuid.UUID, UpdateProfileInput) (*domainuser.User, error)
	FindByEmail(context.Context, string) (*domainuser.User, error)
	// CreatePlaceholder creates an account-less user (no password, no Google link) for an email
	// that was invited to a trip before anyone with that email had signed up. It lets the invited
	// person be assigned as an expense payer/split participant right away; Register or
	// AuthenticateGoogle later claims the same row in place so every reference to its user ID
	// keeps working. If the email is claimed by someone else in the meantime, the existing row
	// wins rather than erroring.
	CreatePlaceholder(ctx context.Context, email, name string) (*domainuser.User, error)
}

type Repository interface {
	Create(context.Context, *domainuser.User) (*domainuser.User, error)
	GetByID(context.Context, uuid.UUID) (*domainuser.User, error)
	GetByEmail(context.Context, string) (*domainuser.User, error)
	GetByGoogleID(context.Context, string) (*domainuser.User, error)
	ExistsByEmail(context.Context, string) (bool, error)
	SetGoogleID(context.Context, uuid.UUID, string) error
	// SetPasswordHash gives a password-less account (Google-only, or a placeholder created by an
	// invite) a password, without touching its other fields.
	SetPasswordHash(context.Context, uuid.UUID, string) error
	Update(context.Context, *domainuser.User) (*domainuser.User, error)
	ListByIDs(context.Context, []uuid.UUID) ([]domainuser.User, error)
}

// GoogleClaims is the identity a verified Google ID token asserts.
type GoogleClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// GoogleVerifier checks a Google-issued ID token's signature, issuer, audience, and expiry.
type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (*GoogleClaims, error)
}

type TokenRepository interface {
	Store(context.Context, domainuser.RefreshToken) error
	GetByHash(context.Context, string) (*domainuser.RefreshToken, error)
	Revoke(context.Context, uuid.UUID) error
	RevokeAllForUser(context.Context, uuid.UUID) error
}

type InvitationFinder interface {
	ListPendingByEmail(context.Context, string) ([]domaininvitation.Invitation, error)
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
