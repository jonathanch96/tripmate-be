package mcp

import (
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	categorydomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense_category"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	invitationdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/invitation"
	oauthdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/oauth"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
)

type Dependencies struct {
	OAuth        oauthdomain.Service
	Trips        tripdomain.Service
	Participants participantdomain.Service
	Expenses     expensedomain.Service
	Categories   categorydomain.Service
	Balances     balancedomain.Service
	Settlements  settlementdomain.Service
	Invitations  invitationdomain.Service
	FX           fxdomain.Service
}

type controller struct {
	deps Dependencies
}
