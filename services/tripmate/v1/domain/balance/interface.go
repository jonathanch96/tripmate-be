package balance

import (
	"context"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/shopspring/decimal"
)

type ExpenseRepository interface {
	ListForBalance(context.Context, uuid.UUID) ([]domainexpense.Expense, error)
}

type SettlementRepository interface {
	ListForBalance(context.Context, uuid.UUID) ([]domainsettlement.Settlement, error)
}

type ParticipantRepository interface {
	ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error)
}

type FXProvider interface {
	EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error)
}

type TripRepository interface {
	GetByID(context.Context, uuid.UUID) (*domaintrip.Trip, error)
}

type Dependencies struct {
	Expenses     ExpenseRepository
	Settlements  SettlementRepository
	Participants ParticipantRepository
	FX           FXProvider
	Trips        TripRepository
}

// LedgerFilter selects whose statement to build and, optionally, narrows every entry down to just
// the net effect between that member and one other (AgainstUserID nil means "against everyone").
type LedgerFilter struct {
	MemberUserID  uuid.UUID
	AgainstUserID *uuid.UUID
}

type Service interface {
	Calculate(context.Context, tripctx.TripContext) (*domainbalance.Result, error)
	Summary(context.Context, tripctx.TripContext) (*domainbalance.Summary, error)
	OutstandingDebt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (decimal.Decimal, error)
	FinalSettlement(context.Context, tripctx.TripContext) (*domainbalance.FinalPlan, error)
	Ledger(context.Context, tripctx.TripContext, LedgerFilter) (*domainbalance.Ledger, error)
}
