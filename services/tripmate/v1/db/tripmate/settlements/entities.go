package settlements

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Settlement struct {
	ID                                             uuid.UUID `gorm:"column:id;primaryKey"`
	TripID                                         uuid.UUID
	FromUserID                                     uuid.UUID
	ToUserID                                       uuid.UUID
	Amount                                         decimal.Decimal `gorm:"type:numeric(20,6)"`
	Currency                                       string
	Method                                         string
	BankName, BankAccountNumber, BankAccountHolder *string
	Note, ProofURL                                 *string
	Status                                         string
	ApprovedByUserID                               *uuid.UUID
	ApprovedAt                                     *time.Time
	RejectedReason                                 *string
	CreatedByUserID                                uuid.UUID
	Version                                        int
	SettlementDate                                 time.Time
	CreatedAt, UpdatedAt                           time.Time
	DeletedAt                                      gorm.DeletedAt
	TotalCount                                     int64   `gorm:"column:total_count;->"`
	FromEmail, FromName, ToEmail, ToName           string  `gorm:"->"`
	FromAvatarURL, ToAvatarURL                     *string `gorm:"->"`
}

func (Settlement) TableName() string { return "tripmate.settlements" }
