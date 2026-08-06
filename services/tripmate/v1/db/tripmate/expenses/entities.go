package expenses

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Expense struct {
	ID               uuid.UUID       `gorm:"column:id;primaryKey"`
	TripID           uuid.UUID       `gorm:"column:trip_id"`
	ExpenseDate      time.Time       `gorm:"column:expense_date;type:date"`
	Description      string          `gorm:"column:description"`
	Amount           decimal.Decimal `gorm:"column:amount;type:numeric(20,6)"`
	Currency         string          `gorm:"column:currency"`
	SplitType        string          `gorm:"column:split_type"`
	Status           string          `gorm:"column:status"`
	Source           string          `gorm:"column:source"`
	Note             *string         `gorm:"column:note"`
	CreatedByUserID  uuid.UUID       `gorm:"column:created_by_user_id"`
	ApprovedByUserID *uuid.UUID      `gorm:"column:approved_by_user_id"`
	ApprovedAt       *time.Time      `gorm:"column:approved_at"`
	RejectedReason   *string         `gorm:"column:rejected_reason"`
	Version          int             `gorm:"column:version"`
	CreatedAt        time.Time       `gorm:"column:created_at"`
	UpdatedAt        time.Time       `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt  `gorm:"column:deleted_at"`
	TotalCount       int64           `gorm:"column:total_count;->"`
}

func (Expense) TableName() string { return "tripmate.expenses" }
