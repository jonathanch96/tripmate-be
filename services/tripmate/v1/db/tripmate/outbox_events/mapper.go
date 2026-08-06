package outbox_events

import (
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
)

func fromDomain(entity event.OutboxEvent) OutboxEvent {
	return OutboxEvent{ID: entity.ID, AggregateType: entity.AggregateType, AggregateID: entity.AggregateID,
		EventType: entity.EventType, Payload: entity.Payload, Status: entity.Status,
		Attempts: entity.Attempts, AvailableAt: entity.AvailableAt, PublishedAt: entity.PublishedAt,
		LastError: entity.LastError, CreatedAt: entity.CreatedAt}
}
