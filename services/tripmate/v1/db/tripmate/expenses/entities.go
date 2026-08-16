package expenses

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Expense struct {
	ID                uuid.UUID        `gorm:"column:id;primaryKey"`
	TripID            uuid.UUID        `gorm:"column:trip_id"`
	CategoryID        *uuid.UUID       `gorm:"column:category_id"`
	ExpenseDate       time.Time        `gorm:"column:expense_date;type:date"`
	Description       string           `gorm:"column:description"`
	Amount            decimal.Decimal  `gorm:"column:amount;type:numeric(20,6)"`
	Currency          string           `gorm:"column:currency"`
	ChargedAmount     *decimal.Decimal `gorm:"column:charged_amount;type:numeric(20,6)"`
	ChargedCurrency   *string          `gorm:"column:charged_currency"`
	SplitType         string           `gorm:"column:split_type"`
	Status            string           `gorm:"column:status"`
	Source            string           `gorm:"column:source"`
	Note              *string          `gorm:"column:note"`
	CreatedByUserID   uuid.UUID        `gorm:"column:created_by_user_id"`
	ApprovedByUserID  *uuid.UUID       `gorm:"column:approved_by_user_id"`
	ApprovedAt        *time.Time       `gorm:"column:approved_at"`
	RejectedReason    *string          `gorm:"column:rejected_reason"`
	Version           int              `gorm:"column:version"`
	CreatedAt         time.Time        `gorm:"column:created_at"`
	UpdatedAt         time.Time        `gorm:"column:updated_at"`
	DeletedAt         gorm.DeletedAt   `gorm:"column:deleted_at"`
	TotalCount        int64            `gorm:"column:total_count;->"`
	CurrencyTotal     decimal.Decimal  `gorm:"column:currency_total;->"`
	StatusCount       int64            `gorm:"column:status_count;->"`
	CurrencyTotals    []byte           `gorm:"column:currency_totals;->"`
	StatusTotals      []byte           `gorm:"column:status_totals;->"`
	CreatorEmail      string           `gorm:"column:creator_email;->"`
	CreatorName       string           `gorm:"column:creator_name;->"`
	CreatorAvatarURL  *string          `gorm:"column:creator_avatar_url;->"`
	CreatorCreatedAt  time.Time        `gorm:"column:creator_created_at;->"`
	CreatorUpdatedAt  time.Time        `gorm:"column:creator_updated_at;->"`
	ApproverEmail     *string          `gorm:"column:approver_email;->"`
	ApproverName      *string          `gorm:"column:approver_name;->"`
	ApproverAvatarURL *string          `gorm:"column:approver_avatar_url;->"`
	ApproverCreatedAt *time.Time       `gorm:"column:approver_created_at;->"`
	ApproverUpdatedAt *time.Time       `gorm:"column:approver_updated_at;->"`
}

func (Expense) TableName() string { return "tripmate.expenses" }
