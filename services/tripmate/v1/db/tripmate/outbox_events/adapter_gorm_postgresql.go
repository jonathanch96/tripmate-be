package outbox_events

import (
	"context"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}
func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) Create(ctx context.Context, entity *event.OutboxEvent) error {
	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}
	if entity.Status == "" {
		entity.Status = "pending"
	}
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Create(ptr(fromDomain(*entity))).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func ptr(value OutboxEvent) *OutboxEvent { return &value }
