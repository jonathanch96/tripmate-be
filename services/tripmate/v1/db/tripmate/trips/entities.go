package trips

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Trip struct {
	ID                  uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	Code                string         `gorm:"column:code"`
	Name                string         `gorm:"column:name"`
	BaseCurrency        string         `gorm:"column:base_currency"`
	Country             *string        `gorm:"column:country"`
	StartDate           time.Time      `gorm:"column:start_date"`
	EndDate             time.Time      `gorm:"column:end_date"`
	PlannerID           uuid.UUID      `gorm:"column:planner_id"`
	IsFinalized         bool           `gorm:"column:is_finalized"`
	FinalizedAt         *time.Time     `gorm:"column:finalized_at"`
	EditPermission      string         `gorm:"column:setting_edit_permission"`
	ApprovalExpenses    bool           `gorm:"column:setting_approval_expenses"`
	ApprovalSettlements bool           `gorm:"column:setting_approval_settlements"`
	MultiCurrency       bool           `gorm:"column:setting_multi_currency_enabled"`
	AllowSettlement     bool           `gorm:"column:setting_allow_settlement_before_end"`
	Version             int            `gorm:"column:version"`
	CreatedAt           time.Time      `gorm:"column:created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (Trip) TableName() string { return "tripmate.trips" }
