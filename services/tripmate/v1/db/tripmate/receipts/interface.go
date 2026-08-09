package receipts

import (
	"context"

	"github.com/google/uuid"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
)

// Repository is this package's own view of what it offers. It is deliberately re-declared rather
// than imported from the domain, so the adapter can be read on its own.
type Repository interface {
	Create(context.Context, *domainreceipt.Receipt) (*domainreceipt.Receipt, error)
	GetByID(context.Context, uuid.UUID) (*domainreceipt.Receipt, error)
	Update(context.Context, *domainreceipt.Receipt) (*domainreceipt.Receipt, error)
	ListByTripID(context.Context, uuid.UUID, receiptdomain.Filter) ([]domainreceipt.Receipt, int64, error)
	SoftDelete(context.Context, uuid.UUID) error
	ReplaceItems(context.Context, uuid.UUID, []domainreceipt.Item) error
	CreateItem(context.Context, *domainreceipt.Item) (*domainreceipt.Item, error)
	UpdateItem(context.Context, *domainreceipt.Item) (*domainreceipt.Item, error)
	GetItem(context.Context, uuid.UUID) (*domainreceipt.Item, error)
	DeleteItem(context.Context, uuid.UUID) error
	ReplaceAssignments(context.Context, uuid.UUID, map[uuid.UUID][]uuid.UUID) error
	ClearExpenseLink(context.Context, uuid.UUID) error
}

var _ Repository = (*adapterGormPostgresql)(nil)
var _ receiptdomain.Repository = (*adapterGormPostgresql)(nil)
