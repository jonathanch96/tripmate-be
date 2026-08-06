package expenses

import (
	"context"

	"github.com/google/uuid"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
)

type Repository interface {
	Create(context.Context, *domainexpense.Expense) (*domainexpense.Expense, error)
	GetByID(context.Context, uuid.UUID) (*domainexpense.Expense, error)
	ListByTripID(context.Context, uuid.UUID, expensedomain.Filter) ([]domainexpense.Expense, int64, expensedomain.Totals, error)
	ListApprovedByTripID(context.Context, uuid.UUID) ([]domainexpense.Expense, error)
	Update(context.Context, *domainexpense.Expense) (*domainexpense.Expense, error)
	SoftDelete(context.Context, uuid.UUID) error
	CountByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	DistinctCurrencies(context.Context, uuid.UUID) ([]string, error)
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ expensedomain.Repository = (*adapterGormPostgresql)(nil)
