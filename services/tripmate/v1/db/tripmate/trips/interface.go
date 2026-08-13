package trips

import (
	"context"

	"github.com/google/uuid"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type Repository interface {
	Create(context.Context, *domaintrip.Trip) (*domaintrip.Trip, error)
	GetByID(context.Context, uuid.UUID) (*domaintrip.Trip, error)
	GetByCode(context.Context, string) (*domaintrip.Trip, error)
	ExistsByCode(context.Context, string) (bool, error)
	ListByUserID(context.Context, uuid.UUID, tripdomain.ListFilter) ([]domaintrip.Trip, int64, error)
	Update(context.Context, *domaintrip.Trip) (*domaintrip.Trip, error)
	SoftDelete(context.Context, uuid.UUID) error
	SetArchived(context.Context, uuid.UUID, bool) error
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ tripdomain.Repository = (*adapterGormPostgresql)(nil)
