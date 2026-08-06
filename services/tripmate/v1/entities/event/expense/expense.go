package expenseevent

import (
	"time"

	"github.com/google/uuid"
)

type Changed struct {
	EventID      uuid.UUID   `json:"event_id"`
	TripID       uuid.UUID   `json:"trip_id"`
	TripCode     string      `json:"trip_code"`
	ExpenseID    uuid.UUID   `json:"expense_id"`
	Amount       string      `json:"amount"`
	Currency     string      `json:"currency"`
	PayerIDs     []uuid.UUID `json:"payer_ids"`
	SplitUserIDs []uuid.UUID `json:"split_user_ids"`
	ActorID      uuid.UUID   `json:"actor_id"`
	OccurredAt   time.Time   `json:"occurred_at"`
}
