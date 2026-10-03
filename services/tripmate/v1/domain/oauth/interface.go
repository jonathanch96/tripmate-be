package oauth

import (
	"context"
	"time"

	"github.com/google/uuid"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type Repository interface {
	CreateClient(context.Context, *domainoauth.Client) (*domainoauth.Client, error)
	GetClientByClientID(context.Context, string) (*domainoauth.Client, error)

	CreateRequest(context.Context, *domainoauth.AuthRequest) (*domainoauth.AuthRequest, error)
	GetRequest(context.Context, uuid.UUID) (*domainoauth.AuthRequest, error)
	GetRequestByCodeHash(context.Context, string) (*domainoauth.AuthRequest, error)
	// UpdateRequestStatus moves a request from one status to another, filling in the decision
	// fields. It reports false when the request was no longer in the expected status, so two
	// concurrent approvals (or two code exchanges) cannot both win.
	UpdateRequestStatus(ctx context.Context, request *domainoauth.AuthRequest, from domainoauth.RequestStatus) (bool, error)

	GetActiveGrant(ctx context.Context, userID, clientID uuid.UUID) (*domainoauth.Grant, error)
	GetGrant(context.Context, uuid.UUID) (*domainoauth.Grant, error)
	CreateGrant(context.Context, *domainoauth.Grant) (*domainoauth.Grant, error)
	UpdateGrantScopes(context.Context, uuid.UUID, []string) error
	TouchGrant(context.Context, uuid.UUID, time.Time) error
	RevokeGrant(context.Context, uuid.UUID) error
	ListActiveGrants(context.Context, uuid.UUID) ([]domainoauth.Grant, error)

	CreateToken(context.Context, *domainoauth.Token) error
	GetTokenByHash(context.Context, string) (*domainoauth.Token, error)
	// RevokeToken marks one token revoked, reporting false if it already was - the refresh path
	// uses this to make rotation single-use under concurrency.
	RevokeToken(context.Context, uuid.UUID, time.Time) (bool, error)
	RevokeTokensForGrant(context.Context, uuid.UUID) error
}

// UserFinder loads the account a grant belongs to, so the MCP layer can build an identity.
type UserFinder interface {
	GetByID(context.Context, uuid.UUID) (*domainuser.User, error)
}

type UnitOfWork interface {
	Do(context.Context, func(context.Context) error) error
}

type Config struct {
	// Issuer is the public origin the AI tools see, e.g. https://tripmate.example.com. It is the
	// OAuth issuer and the prefix of every endpoint URL advertised in metadata.
	Issuer string
	// ConsentURL is the frontend page that shows a pending request to the signed-in user.
	ConsentURL string
	// AccessTTL, RefreshTTL, CodeTTL, RequestTTL and GrantIdleTTL default when zero.
	AccessTTL, RefreshTTL, CodeTTL, RequestTTL, GrantIdleTTL time.Duration
}

type Dependencies struct {
	Repo   Repository
	Users  UserFinder
	UOW    UnitOfWork
	Config Config
	Clock  func() time.Time
}

type RegisterInput struct {
	ClientName              string
	ClientURI               string
	LogoURI                 string
	RedirectURIs            []string
	TokenEndpointAuthMethod string
	GrantTypes              []string
	ResponseTypes           []string
}

// RegisteredClient is the registration response: the stored client plus the one-time plaintext
// secret for confidential clients.
type RegisteredClient struct {
	Client       domainoauth.Client
	ClientSecret string
}

type AuthorizeInput struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
}

// AuthorizeResult is where /oauth/authorize sends the browser next: the consent page for a valid
// request, or straight back to the client with an error.
type AuthorizeResult struct {
	RedirectURL string
}

type TokenInput struct {
	GrantType    string
	Code         string
	RedirectURI  string
	CodeVerifier string
	RefreshToken string
	ClientID     string
	ClientSecret string
	Resource     string
	Scope        string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Scopes       []string
}

// AccessInfo is what a verified MCP bearer token stands for.
type AccessInfo struct {
	User       domainuser.User
	GrantID    uuid.UUID
	ClientName string
	Scopes     []string
	ExpiresAt  time.Time
}

// RequestView is what the consent page shows about a pending request.
type RequestView struct {
	Request domainoauth.AuthRequest
	Client  domainoauth.Client
}

type Service interface {
	RegisterClient(context.Context, RegisterInput) (*RegisteredClient, error)
	Authorize(context.Context, AuthorizeInput) (*AuthorizeResult, error)
	GetRequest(ctx context.Context, userID, requestID uuid.UUID) (*RequestView, error)
	Approve(ctx context.Context, userID, requestID uuid.UUID, scopes []string) (string, error)
	Deny(ctx context.Context, userID, requestID uuid.UUID) (string, error)
	Token(context.Context, TokenInput) (*TokenPair, error)
	VerifyAccess(context.Context, string) (*AccessInfo, error)
	ListGrants(context.Context, uuid.UUID) ([]domainoauth.Grant, error)
	RevokeGrant(ctx context.Context, userID, grantID uuid.UUID) error
	// Resource is the canonical URL of the protected MCP endpoint.
	Resource() string
	Issuer() string
}
