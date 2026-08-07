package exchange_rates

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type ExchangeRate struct {
	ID                   uuid.UUID       `gorm:"column:id;primaryKey"`
	TripID               *uuid.UUID      `gorm:"column:trip_id"`
	FromCurrency         string          `gorm:"column:from_currency"`
	ToCurrency           string          `gorm:"column:to_currency"`
	Rate                 decimal.Decimal `gorm:"column:rate;type:numeric(24,12)"`
	IsFinal              bool            `gorm:"column:is_final"`
	Source               string          `gorm:"column:source"`
	EffectiveAt          time.Time       `gorm:"column:effective_at"`
	CreatedAt, UpdatedAt time.Time
}

func (ExchangeRate) TableName() string { return "tripmate.exchange_rates" }
