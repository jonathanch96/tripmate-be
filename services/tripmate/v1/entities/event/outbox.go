package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type OutboxEvent struct {
	ID                       uuid.UUID
	AggregateType, EventType string
	AggregateID              uuid.UUID
	Payload                  json.RawMessage
	Status                   string
	Attempts                 int
	AvailableAt              time.Time
	PublishedAt              *time.Time
	LastError                *string
	CreatedAt                time.Time
}
