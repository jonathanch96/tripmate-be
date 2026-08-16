package settlements

import (
	"context"

	"github.com/google/uuid"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
)

type Repository interface {
	Create(context.Context, *domainsettlement.Settlement) (*domainsettlement.Settlement, error)
	GetByID(context.Context, uuid.UUID) (*domainsettlement.Settlement, error)
	Update(context.Context, *domainsettlement.Settlement) (*domainsettlement.Settlement, error)
	ListByTripID(context.Context, uuid.UUID, settlementdomain.Filter) ([]domainsettlement.Settlement, int64, error)
	ListApprovedByTripID(context.Context, uuid.UUID) ([]domainsettlement.Settlement, error)
	ListForBalance(context.Context, uuid.UUID) ([]domainsettlement.Settlement, error)
	SoftDelete(context.Context, uuid.UUID) error
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ settlementdomain.Repository = (*adapterGormPostgresql)(nil)
var _ balancedomain.SettlementRepository = (*adapterGormPostgresql)(nil)
