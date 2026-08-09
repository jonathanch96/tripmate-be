package receipt

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusUploaded   Status = "uploaded"
	StatusExtracting Status = "extracting"
	StatusExtracted  Status = "extracted"
	StatusFailed     Status = "failed"
	StatusConverted  Status = "converted"
)

// transitions is the receipt lifecycle: uploaded → extracting → extracted → converted, with failed
// reachable from extracting and retryable back into it. Anything not listed here is illegal.
var transitions = map[Status][]Status{
	StatusUploaded:   {StatusExtracting},
	StatusExtracting: {StatusExtracted, StatusFailed},
	StatusExtracted:  {StatusExtracting, StatusConverted},
	StatusFailed:     {StatusExtracting},
	StatusConverted:  {StatusExtracted},
}

func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// AssertTransition reports whether the receipt may move to next, naming both ends when it may not.
func (s Status) AssertTransition(next Status) error {
	if !s.CanTransitionTo(next) {
		return apperror.Newf("VALIDATION_FAILED", "a %s receipt cannot become %s", s, next)
	}
	return nil
}

type Item struct {
	ID, ReceiptID uuid.UUID
	Name          string
	Quantity      decimal.Decimal
	UnitPrice     decimal.Decimal
	TotalPrice    decimal.Decimal
	SortOrder     int
	// Assignees are the diners sharing this line. Empty means nobody has claimed it yet, which
	// blocks conversion (rule A-3).
	Assignees            []uuid.UUID
	CreatedAt, UpdatedAt time.Time
}

type Receipt struct {
	ID, TripID    uuid.UUID
	ExpenseID     *uuid.UUID
	UploadedBy    uuid.UUID
	StorageKey    string
	ContentType   string
	SizeBytes     int64
	Merchant      *string
	ReceiptDate   *time.Time
	Currency      *string
	Subtotal      *decimal.Decimal
	Tax           *decimal.Decimal
	ServiceCharge *decimal.Decimal
	Total         *decimal.Decimal
	Provider      *string
	ProviderModel *string
	RawResponse   json.RawMessage
	Status        Status
	FailureReason *string
	Items         []Item
	Uploader      *domainuser.PublicUser
	// ImageURL is a short-lived presigned link minted per request; it is never persisted.
	ImageURL             string
	CreatedAt, UpdatedAt time.Time
}

// Editable reports whether the receipt's contents may still be changed. A converted receipt is a
// record of what produced an expense, so it is read-only (rule R-5).
func (r Receipt) Editable() error {
	if r.Status == StatusConverted {
		return apperror.Newf("VALIDATION_FAILED", "a converted receipt is read-only; delete its expense to reopen it")
	}
	return nil
}

// EditableBy is rule R-6: only the person who uploaded the receipt or a planner may change it.
func (r Receipt) EditableBy(actorID uuid.UUID, planner bool) error {
	if planner || r.UploadedBy == actorID {
		return nil
	}
	return apperror.New("FORBIDDEN")
}

// HasAssignments reports whether any line has been claimed. Re-extraction throws items away, so it
// is refused once assignments exist (rule R-2).
func (r Receipt) HasAssignments() bool {
	for _, item := range r.Items {
		if len(item.Assignees) > 0 {
			return true
		}
	}
	return false
}

// Extras is the part of the bill that is not a line item — tax plus service charge.
func (r Receipt) Extras() decimal.Decimal {
	return value(r.Tax).Add(value(r.ServiceCharge))
}

// ItemsTotal is the sum of every line, assigned or not.
func (r Receipt) ItemsTotal() decimal.Decimal {
	total := decimal.Zero
	for _, item := range r.Items {
		total = total.Add(item.TotalPrice)
	}
	return total
}

// AmountDue is what the receipt says is payable: the parsed total when the model found one,
// otherwise items plus extras (rule OCR-5 applied at read time).
func (r Receipt) AmountDue() decimal.Decimal {
	if r.Total != nil && !r.Total.IsZero() {
		return *r.Total
	}
	return r.ItemsTotal().Add(r.Extras())
}

// UnassignedItems lists the lines nobody has claimed, in display order.
func (r Receipt) UnassignedItems() []Item {
	var rows []Item
	for _, item := range r.Items {
		if len(item.Assignees) == 0 {
			rows = append(rows, item)
		}
	}
	return rows
}

func value(amount *decimal.Decimal) decimal.Decimal {
	if amount == nil {
		return decimal.Zero
	}
	return *amount
}
