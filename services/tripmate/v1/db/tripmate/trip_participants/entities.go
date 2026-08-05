package trip_participants

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Participant struct {
	ID                uuid.UUID `gorm:"column:id;primaryKey"`
	TripID            uuid.UUID `gorm:"column:trip_id"`
	UserID            uuid.UUID `gorm:"column:user_id"`
	Role              string    `gorm:"column:role"`
	BankName          *string   `gorm:"column:bank_name"`
	BankAccountNumber *string   `gorm:"column:bank_account_number"`
	BankAccountHolder *string   `gorm:"column:bank_account_holder"`
	JoinedAt          time.Time `gorm:"column:joined_at"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt
}

func (Participant) TableName() string { return "tripmate.trip_participants" }

type participantWithUser struct {
	Participant
	Email         string
	Name          string
	AvatarURL     *string
	UserCreatedAt time.Time
	UserUpdatedAt time.Time
}
