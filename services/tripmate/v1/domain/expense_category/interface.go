package expense_category

import (
	"context"

	"github.com/google/uuid"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
)

type Repository interface {
	ListForTrip(context.Context, uuid.UUID) ([]domaincat.ExpenseCategory, error)
	GetByID(context.Context, uuid.UUID) (*domaincat.ExpenseCategory, error)
	ExistsByName(context.Context, *uuid.UUID, string) (bool, error)
	Create(context.Context, *domaincat.ExpenseCategory) (*domaincat.ExpenseCategory, error)
	Delete(context.Context, uuid.UUID) error
}

type Service interface {
	List(context.Context, uuid.UUID) ([]domaincat.ExpenseCategory, error)
	Create(context.Context, uuid.UUID, string) (*domaincat.ExpenseCategory, error)
	Delete(context.Context, uuid.UUID, uuid.UUID) error
}
