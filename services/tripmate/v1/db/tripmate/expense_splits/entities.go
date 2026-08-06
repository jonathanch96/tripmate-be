package expense_splits

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Split struct {
	ID            uuid.UUID       `gorm:"column:id;primaryKey"`
	ExpenseID     uuid.UUID       `gorm:"column:expense_id"`
	UserID        uuid.UUID       `gorm:"column:user_id"`
	Amount        decimal.Decimal `gorm:"column:amount;type:numeric(20,6)"`
	CreatedAt     time.Time       `gorm:"column:created_at"`
	UserEmail     string          `gorm:"column:user_email;->"`
	UserName      string          `gorm:"column:user_name;->"`
	UserAvatarURL *string         `gorm:"column:user_avatar_url;->"`
	UserCreatedAt time.Time       `gorm:"column:user_created_at;->"`
	UserUpdatedAt time.Time       `gorm:"column:user_updated_at;->"`
}

func (Split) TableName() string { return "tripmate.expense_splits" }
