package oauth

import (
	"time"

	"github.com/google/uuid"
)

// Scopes a connected AI tool can be granted. Write implies read: a grant holding ScopeWrite is
// always stored with ScopeRead too.
const (
	ScopeRead  = "tripmate.read"
	ScopeWrite = "tripmate.write"
)

// SupportedScopes is the full catalogue, in the order the consent page lists them.
var SupportedScopes = []string{ScopeRead, ScopeWrite}

type AuthMethod string

const (
	AuthMethodNone  AuthMethod = "none"
	AuthMethodPost  AuthMethod = "client_secret_post"
	AuthMethodBasic AuthMethod = "client_secret_basic"
)

// Client is an AI tool registered through dynamic client registration (RFC 7591).
type Client struct {
	ID               uuid.UUID
	ClientID         string
	ClientSecretHash *string
	Name             string
	ClientURI        *string
	LogoURI          *string
	RedirectURIs     []string
	AuthMethod       AuthMethod
	CreatedAt        time.Time
}

type RequestStatus string

const (
	RequestPending  RequestStatus = "pending"
	RequestApproved RequestStatus = "approved"
	RequestDenied   RequestStatus = "denied"
	RequestUsed     RequestStatus = "used"
)

// AuthRequest is one /oauth/authorize visit waiting for the user on the consent page. Approving it
// mints the authorization code onto the same record.
type AuthRequest struct {
	ID            uuid.UUID
	ClientID      uuid.UUID
	RedirectURI   string
	Scopes        []string
	State         *string
	CodeChallenge string
	Resource      *string
	Status        RequestStatus
	UserID        *uuid.UUID
	GrantedScopes []string
	CodeHash      *string
	CodeExpiresAt *time.Time
	ExpiresAt     time.Time
	CreatedAt     time.Time
	// Client is populated by reads that join the registering client.
	Client *Client
}

// Grant is one connected app: a user's standing approval of a client.
type Grant struct {
	ID         uuid.UUID
	ClientID   uuid.UUID
	UserID     uuid.UUID
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt time.Time
	RevokedAt  *time.Time
	Client     *Client
}

type TokenKind string

const (
	TokenAccess  TokenKind = "access"
	TokenRefresh TokenKind = "refresh"
)

type Token struct {
	ID        uuid.UUID
	GrantID   uuid.UUID
	Kind      TokenKind
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// HasScope reports whether scopes contains want.
func HasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}
