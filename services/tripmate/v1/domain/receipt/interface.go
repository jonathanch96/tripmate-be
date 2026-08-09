package receipt

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	"github.com/shopspring/decimal"
)

// Image is the picture handed to an OCR provider, already sniffed and stripped of metadata.
type Image struct {
	Data        []byte
	ContentType string
}

// ExtractedItem is one line the model claims to have read. Flagged marks a line whose numbers came
// back malformed: rather than failing the whole receipt, the amount is zeroed and the line is
// marked for a human to fix (rule OCR-3).
type ExtractedItem struct {
	Name       string
	Quantity   decimal.Decimal
	UnitPrice  decimal.Decimal
	TotalPrice decimal.Decimal
	Flagged    bool
}

type ExtractionResult struct {
	Merchant                            string
	Date                                *time.Time
	Currency                            string
	Items                               []ExtractedItem
	Subtotal, Tax, ServiceCharge, Total decimal.Decimal
	Raw                                 json.RawMessage
	// Discrepancy is rule OCR-4: the numbers do not reconcile, but the result is still usable and
	// the UI should ask a human about it.
	Discrepancy bool
}

// OCRProvider is the port an image-to-text model sits behind. It is deliberately narrow: swapping
// Gemini for another model, or for the offline noop used in CI, touches nothing else.
type OCRProvider interface {
	Extract(ctx context.Context, img Image) (*ExtractionResult, error)
	Name() string
	Model() string
}

// ObjectStorage mirrors pkg/storage's port. It is re-declared here so the domain depends on its own
// abstraction rather than on a package that knows about S3.
type ObjectStorage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Delete(ctx context.Context, key string) error
}

type Repository interface {
	Create(context.Context, *domainreceipt.Receipt) (*domainreceipt.Receipt, error)
	GetByID(context.Context, uuid.UUID) (*domainreceipt.Receipt, error)
	Update(context.Context, *domainreceipt.Receipt) (*domainreceipt.Receipt, error)
	ListByTripID(context.Context, uuid.UUID, Filter) ([]domainreceipt.Receipt, int64, error)
	SoftDelete(context.Context, uuid.UUID) error
	ReplaceItems(context.Context, uuid.UUID, []domainreceipt.Item) error
	CreateItem(context.Context, *domainreceipt.Item) (*domainreceipt.Item, error)
	UpdateItem(context.Context, *domainreceipt.Item) (*domainreceipt.Item, error)
	GetItem(context.Context, uuid.UUID) (*domainreceipt.Item, error)
	DeleteItem(context.Context, uuid.UUID) error
	ReplaceAssignments(context.Context, uuid.UUID, map[uuid.UUID][]uuid.UUID) error
	// ClearExpenseLink returns every receipt pointing at an expense to `extracted` (rule C-7).
	ClearExpenseLink(context.Context, uuid.UUID) error
}

type ParticipantRepository interface {
	ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error)
}

// ExpenseCreator is the slice of the expense domain this one needs. Depending on the interface
// rather than the concrete service keeps the two domains free of a cycle.
type ExpenseCreator interface {
	Create(context.Context, identity.Identity, tripctx.TripContext, expensedomain.CreateInput) (*domainexpense.Expense, error)
}

type UnitOfWork interface {
	Do(context.Context, func(context.Context) error) error
}

type Filter struct {
	Page, PerPage int
	Status        *domainreceipt.Status
}

type UploadInput struct {
	Data        []byte
	ContentType string
	Filename    string
}

type UpdateItemInput struct {
	Name       *string
	Quantity   *decimal.Decimal
	UnitPrice  *decimal.Decimal
	TotalPrice *decimal.Decimal
}

type AddItemInput struct {
	Name       string
	Quantity   decimal.Decimal
	UnitPrice  decimal.Decimal
	TotalPrice decimal.Decimal
}

type ItemAssignmentInput struct {
	ItemID  uuid.UUID
	UserIDs []uuid.UUID
}

// UserShare is what one diner owes: what they ate, their slice of tax and service, and the total.
type UserShare struct {
	UserID        uuid.UUID
	ItemsSubtotal decimal.Decimal
	Tax           decimal.Decimal
	ServiceCharge decimal.Decimal
	Total         decimal.Decimal
}

type ShareBreakdown struct {
	Currency        string
	ItemsTotal      decimal.Decimal
	Tax             decimal.Decimal
	ServiceCharge   decimal.Decimal
	Total           decimal.Decimal
	PerUser         []UserShare
	UnassignedItems []uuid.UUID
	// Discrepancy is rule A-6: the lines do not add up to the printed subtotal. Conversion is still
	// allowed, because the assignments are what actually decide who owes what.
	Discrepancy bool
}

type PayerInput struct {
	UserID uuid.UUID
	Amount decimal.Decimal
}

type ConvertInput struct {
	Description string
	ExpenseDate *time.Time
	Payers      []PayerInput
}

type Service interface {
	Upload(context.Context, identity.Identity, tripctx.TripContext, UploadInput) (*domainreceipt.Receipt, error)
	Extract(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) (*domainreceipt.Receipt, error)
	Get(context.Context, tripctx.TripContext, uuid.UUID) (*domainreceipt.Receipt, error)
	ListByTrip(context.Context, tripctx.TripContext, Filter) ([]domainreceipt.Receipt, int64, error)
	AddItem(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, AddItemInput) (*domainreceipt.Item, error)
	UpdateItem(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, UpdateItemInput) (*domainreceipt.Item, error)
	DeleteItem(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) error
	SetAssignments(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, []ItemAssignmentInput) error
	Shares(context.Context, tripctx.TripContext, uuid.UUID) (*ShareBreakdown, error)
	ConvertToExpense(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, ConvertInput) (*domainexpense.Expense, error)
	Delete(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) error
}
