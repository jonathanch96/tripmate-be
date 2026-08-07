package outbox_events

import (
	"context"

	"github.com/google/uuid"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
)

type Repository interface {
	Create(context.Context, *event.OutboxEvent) error
	CountByAggregateID(context.Context, uuid.UUID) (int64, error)
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ expensedomain.OutboxRepository = (*adapterGormPostgresql)(nil)
