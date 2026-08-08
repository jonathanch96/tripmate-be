package finance

import (
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	finaldomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/finalization"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
)

type controller struct {
	trips       tripdomain.Service
	parts       participantdomain.Service
	balances    balancedomain.Service
	settlements settlementdomain.Service
	fx          fxdomain.Service
	final       finaldomain.Service
}
