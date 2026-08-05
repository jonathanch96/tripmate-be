package trip_invitations

import (
	"time"

	"github.com/google/uuid"
)

type Invitation struct {
	ID         uuid.UUID  `gorm:"column:id;primaryKey"`
	TripID     uuid.UUID  `gorm:"column:trip_id"`
	Email      string     `gorm:"column:email"`
	Token      string     `gorm:"column:token"`
	Status     string     `gorm:"column:status"`
	InvitedBy  uuid.UUID  `gorm:"column:invited_by"`
	ExpiresAt  time.Time  `gorm:"column:expires_at"`
	AcceptedAt *time.Time `gorm:"column:accepted_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (Invitation) TableName() string { return "tripmate.trip_invitations" }
