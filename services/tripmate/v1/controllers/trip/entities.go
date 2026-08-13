package trip

import (
	"context"

	"github.com/jblabs/tripmate-be/pkg/tripctx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
)

type BalanceService interface {
	Calculate(context.Context, tripctx.TripContext) (*domainbalance.Result, error)
}

type controller struct {
	trips    tripdomain.Service
	parts    participantdomain.Service
	balances BalanceService
}
