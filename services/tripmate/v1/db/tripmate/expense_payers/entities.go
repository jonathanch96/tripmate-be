package expense_payers

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Payer struct {
	ID        uuid.UUID       `gorm:"column:id;primaryKey"`
	ExpenseID uuid.UUID       `gorm:"column:expense_id"`
	UserID    uuid.UUID       `gorm:"column:user_id"`
	Amount    decimal.Decimal `gorm:"column:amount;type:numeric(20,6)"`
	CreatedAt time.Time       `gorm:"column:created_at"`
}

func (Payer) TableName() string { return "tripmate.expense_payers" }
