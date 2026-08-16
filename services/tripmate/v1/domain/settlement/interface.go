package settlement

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	shared "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/shared"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	"github.com/shopspring/decimal"
)

type Filter struct {
	Status        *domainsettlement.Status
	ParticipantID *uuid.UUID
	Method        *domainsettlement.Method
	Page, PerPage int
}

type RecordInput struct {
	FromUserID, ToUserID uuid.UUID
	Amount               decimal.Decimal
	Currency             string
	Method               domainsettlement.Method
	Note, ProofURL       *string
}

type Repository interface {
	Create(context.Context, *domainsettlement.Settlement) (*domainsettlement.Settlement, error)
	GetByID(context.Context, uuid.UUID) (*domainsettlement.Settlement, error)
	Update(context.Context, *domainsettlement.Settlement) (*domainsettlement.Settlement, error)
	ListByTripID(context.Context, uuid.UUID, Filter) ([]domainsettlement.Settlement, int64, error)
	ListForBalance(context.Context, uuid.UUID) ([]domainsettlement.Settlement, error)
	SoftDelete(context.Context, uuid.UUID) error
}
type ParticipantRepository interface {
	GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error)
}
type OutboxRepository interface {
	Create(context.Context, *event.OutboxEvent) error
}

type Dependencies struct {
	Repo         Repository
	Participants ParticipantRepository
	Outbox       OutboxRepository
	UOW          shared.UnitOfWork
	Clock        func() time.Time
}

type Service interface {
	Record(context.Context, identity.Identity, tripctx.TripContext, RecordInput) (*domainsettlement.Settlement, error)
	Approve(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) (*domainsettlement.Settlement, error)
	Reject(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, string) (*domainsettlement.Settlement, error)
	List(context.Context, tripctx.TripContext, Filter) ([]domainsettlement.Settlement, int64, error)
	Delete(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) error
}
