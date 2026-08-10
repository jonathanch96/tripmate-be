package expense

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	shared "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/shared"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
)

type Repository interface {
	Create(context.Context, *domainexpense.Expense) (*domainexpense.Expense, error)
	GetByID(context.Context, uuid.UUID) (*domainexpense.Expense, error)
	ListByTripID(context.Context, uuid.UUID, Filter) ([]domainexpense.Expense, int64, Totals, error)
	ListApprovedByTripID(context.Context, uuid.UUID) ([]domainexpense.Expense, error)
	Update(context.Context, *domainexpense.Expense) (*domainexpense.Expense, error)
	SoftDelete(context.Context, uuid.UUID) error
	CountByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	DistinctCurrencies(context.Context, uuid.UUID) ([]string, error)
}

type PayerRepository interface {
	BulkCreate(context.Context, uuid.UUID, []domainexpense.Payer) error
	ListByExpenseIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]domainexpense.Payer, error)
	DeleteByExpenseID(context.Context, uuid.UUID) error
}

type SplitRepository interface {
	BulkCreate(context.Context, uuid.UUID, []domainexpense.Split) error
	ListByExpenseIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]domainexpense.Split, error)
	DeleteByExpenseID(context.Context, uuid.UUID) error
}

type ParticipantRepository interface {
	ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error)
}

type OutboxRepository interface {
	Create(context.Context, *event.OutboxEvent) error
}

type ReceiptLinker interface {
	ClearExpenseLink(context.Context, uuid.UUID) error
}

type CategoryFinder interface {
	GetByID(context.Context, uuid.UUID) (*domaincat.ExpenseCategory, error)
}

type Dependencies struct {
	Expenses     Repository
	Payers       PayerRepository
	Splits       SplitRepository
	Participants ParticipantRepository
	Outbox       OutboxRepository
	Receipts     ReceiptLinker
	Categories   CategoryFinder
	UOW          shared.UnitOfWork
	Clock        func() time.Time
}

type Service interface {
	Create(context.Context, identity.Identity, tripctx.TripContext, CreateInput) (*domainexpense.Expense, error)
	Update(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, UpdateInput) (*domainexpense.Expense, error)
	Delete(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) error
	Approve(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) (*domainexpense.Expense, error)
	Reject(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, string) (*domainexpense.Expense, error)
	List(context.Context, tripctx.TripContext, Filter) ([]domainexpense.Expense, int64, Totals, error)
	Get(context.Context, tripctx.TripContext, uuid.UUID) (*domainexpense.Expense, error)
}
