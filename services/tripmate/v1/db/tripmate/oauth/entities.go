package oauth

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Client struct {
	ID                      uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	ClientID                string         `gorm:"column:client_id;not null"`
	ClientSecretHash        *string        `gorm:"column:client_secret_hash"`
	Name                    string         `gorm:"column:name;not null"`
	ClientURI               *string        `gorm:"column:client_uri"`
	LogoURI                 *string        `gorm:"column:logo_uri"`
	RedirectURIs            pq.StringArray `gorm:"column:redirect_uris;type:text[];not null"`
	TokenEndpointAuthMethod string         `gorm:"column:token_endpoint_auth_method;not null"`
	CreatedAt               time.Time      `gorm:"column:created_at"`
}

func (Client) TableName() string { return "tripmate.oauth_clients" }

type AuthRequest struct {
	ID            uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	ClientID      uuid.UUID      `gorm:"column:client_id;type:uuid;not null"`
	RedirectURI   string         `gorm:"column:redirect_uri;not null"`
	Scopes        pq.StringArray `gorm:"column:scopes;type:text[];not null"`
	State         *string        `gorm:"column:state"`
	CodeChallenge string         `gorm:"column:code_challenge;not null"`
	Resource      *string        `gorm:"column:resource"`
	Status        string         `gorm:"column:status;not null"`
	UserID        *uuid.UUID     `gorm:"column:user_id;type:uuid"`
	GrantedScopes pq.StringArray `gorm:"column:granted_scopes;type:text[]"`
	CodeHash      *string        `gorm:"column:code_hash"`
	CodeExpiresAt *time.Time     `gorm:"column:code_expires_at"`
	ExpiresAt     time.Time      `gorm:"column:expires_at;not null"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
}

func (AuthRequest) TableName() string { return "tripmate.oauth_auth_requests" }

type Grant struct {
	ID         uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	ClientID   uuid.UUID      `gorm:"column:client_id;type:uuid;not null"`
	UserID     uuid.UUID      `gorm:"column:user_id;type:uuid;not null"`
	Scopes     pq.StringArray `gorm:"column:scopes;type:text[];not null"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
	LastUsedAt time.Time      `gorm:"column:last_used_at"`
	RevokedAt  *time.Time     `gorm:"column:revoked_at"`
}

func (Grant) TableName() string { return "tripmate.oauth_grants" }

type Token struct {
	ID        uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	GrantID   uuid.UUID  `gorm:"column:grant_id;type:uuid;not null"`
	Kind      string     `gorm:"column:kind;not null"`
	TokenHash string     `gorm:"column:token_hash;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (Token) TableName() string { return "tripmate.oauth_tokens" }
