package fx

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/domain/shared"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	"github.com/shopspring/decimal"
)

type SetRateInput struct {
	FromCurrency, ToCurrency string
	Rate                     decimal.Decimal
}

type Dependencies struct {
	Repo  Repository
	UOW   shared.UnitOfWork
	Clock func() time.Time
}

type Service interface {
	EffectiveTable(context.Context, uuid.UUID) (*RateTable, error)
	SetTripRate(context.Context, identity.Identity, tripctx.TripContext, SetRateInput) (*domainfx.Rate, error)
	ListForTrip(context.Context, tripctx.TripContext) ([]domainfx.Rate, error)
	LockAll(context.Context, uuid.UUID) error
	ListGlobal(context.Context) ([]domainfx.Rate, error)
}

type Repository interface {
	ListEffective(context.Context, uuid.UUID) ([]domainfx.Rate, error)
	Get(context.Context, *uuid.UUID, string, string) (*domainfx.Rate, error)
	Upsert(context.Context, domainfx.Rate) (*domainfx.Rate, error)
	// DeleteTripPair removes one trip-scoped direction, leaving global rows untouched.
	DeleteTripPair(ctx context.Context, tripID uuid.UUID, from, to string) error
	ListGlobal(context.Context) ([]domainfx.Rate, error)
}
