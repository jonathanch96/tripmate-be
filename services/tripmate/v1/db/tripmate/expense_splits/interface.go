package expense_splits

import (
	"context"

	"github.com/google/uuid"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
)

type Repository interface {
	BulkCreate(context.Context, uuid.UUID, []domainexpense.Split) error
	ListByExpenseIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]domainexpense.Split, error)
	DeleteByExpenseID(context.Context, uuid.UUID) error
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ expensedomain.SplitRepository = (*adapterGormPostgresql)(nil)
