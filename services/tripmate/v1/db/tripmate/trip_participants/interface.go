package trip_participants

import (
	"context"

	"github.com/google/uuid"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
)

type Repository interface {
	Create(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error)
	GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error)
	GetByID(context.Context, uuid.UUID) (*domainparticipant.Participant, error)
	ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error)
	Update(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error)
	SoftDelete(context.Context, uuid.UUID) error
	CountPlanners(context.Context, uuid.UUID) (int64, error)
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ participantdomain.Repository = (*adapterGormPostgresql)(nil)
