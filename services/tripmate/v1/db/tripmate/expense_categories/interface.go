package expense_categories

import (
	"context"

	"github.com/google/uuid"
	categorydomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense_category"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
)

type Repository interface {
	ListForTrip(context.Context, uuid.UUID) ([]domaincat.ExpenseCategory, error)
	GetByID(context.Context, uuid.UUID) (*domaincat.ExpenseCategory, error)
	ExistsByName(context.Context, *uuid.UUID, string) (bool, error)
	Create(context.Context, *domaincat.ExpenseCategory) (*domaincat.ExpenseCategory, error)
	Delete(context.Context, uuid.UUID) error
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ categorydomain.Repository = (*adapterGormPostgresql)(nil)
