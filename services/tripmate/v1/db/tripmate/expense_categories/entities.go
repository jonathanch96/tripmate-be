package expense_categories

import (
	"time"

	"github.com/google/uuid"
)

type ExpenseCategory struct {
	ID        uuid.UUID  `gorm:"column:id;primaryKey"`
	TripID    *uuid.UUID `gorm:"column:trip_id"`
	Name      string     `gorm:"column:name"`
	IsDefault bool       `gorm:"column:is_default"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (ExpenseCategory) TableName() string { return "tripmate.expense_categories" }
