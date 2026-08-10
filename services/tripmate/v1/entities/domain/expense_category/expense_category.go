package expense_category

import (
	"time"

	"github.com/google/uuid"
)

// ExpenseCategory is a per-trip reference list entry. A nil TripID means a global default
// (seeded once, shared by every trip); a set TripID means a custom category added for that trip.
type ExpenseCategory struct {
	ID        uuid.UUID
	TripID    *uuid.UUID
	Name      string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
