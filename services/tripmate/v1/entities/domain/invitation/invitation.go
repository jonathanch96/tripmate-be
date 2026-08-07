package invitation

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"
	StatusRevoked  Status = "revoked"
	StatusExpired  Status = "expired"
)

type Invitation struct {
	ID, TripID, InvitedBy uuid.UUID
	Email, Token          string
	Status                Status
	ExpiresAt             time.Time
	AcceptedAt            *time.Time
	CreatedAt, UpdatedAt  time.Time
}
