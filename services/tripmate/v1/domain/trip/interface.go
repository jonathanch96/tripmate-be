package trip

import (
	"context"

	"github.com/google/uuid"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

// ListFilter.Archived is nil for "both", true for archived-only, false for active-only.
type ListFilter struct {
	Page, PerPage int
	Archived      *bool
}
type Repository interface {
	Create(context.Context, *domaintrip.Trip) (*domaintrip.Trip, error)
	GetByID(context.Context, uuid.UUID) (*domaintrip.Trip, error)
	GetByCode(context.Context, string) (*domaintrip.Trip, error)
	ExistsByCode(context.Context, string) (bool, error)
	ListByUserID(context.Context, uuid.UUID, ListFilter) ([]domaintrip.Trip, int64, error)
	Update(context.Context, *domaintrip.Trip) (*domaintrip.Trip, error)
	SetArchived(context.Context, uuid.UUID, bool) error
	SoftDelete(context.Context, uuid.UUID) error
}
type FXProvider interface {
	EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error)
}
type ParticipantRepository interface {
	Create(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error)
	GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error)
}
type Transactor interface {
	CreateTripWithPlanner(context.Context, *domaintrip.Trip, *domainparticipant.Participant) (*domaintrip.Trip, error)
}
type ExpenseCounter interface {
	CountByTrip(context.Context, uuid.UUID) (int64, error)
	CurrenciesByTrip(context.Context, uuid.UUID) ([]string, error)
}
type CodeGenerator interface{ Generate() (string, error) }
type Service interface {
	Create(context.Context, uuid.UUID, CreateInput) (*domaintrip.Trip, error)
	GetByCode(context.Context, uuid.UUID, string) (*domaintrip.Trip, error)
	ListMine(context.Context, uuid.UUID, ListFilter) ([]domaintrip.Trip, int64, error)
	UpdateSettings(context.Context, uuid.UUID, string, UpdateSettingsInput) (*domaintrip.Trip, error)
	FindByCode(context.Context, string) (*domaintrip.Trip, error)
	Archive(context.Context, uuid.UUID, string) (*domaintrip.Trip, error)
	Unarchive(context.Context, uuid.UUID, string) (*domaintrip.Trip, error)
}
