package trip_invitations

import (
	"context"

	"github.com/google/uuid"
	invitationdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/invitation"
	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
)

type Repository interface {
	Create(context.Context, *domaininvitation.Invitation) (*domaininvitation.Invitation, error)
	Update(context.Context, *domaininvitation.Invitation) (*domaininvitation.Invitation, error)
	GetByToken(context.Context, string) (*domaininvitation.Invitation, error)
	GetPending(context.Context, uuid.UUID, string) (*domaininvitation.Invitation, error)
	ListPendingByEmail(context.Context, string) ([]domaininvitation.Invitation, error)
	ListByTrip(context.Context, uuid.UUID) ([]domaininvitation.Invitation, error)
	Revoke(context.Context, uuid.UUID) error
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ invitationdomain.Repository = (*adapterGormPostgresql)(nil)
