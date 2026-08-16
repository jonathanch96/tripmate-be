package expense

import (
	"time"

	"github.com/google/uuid"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
)

type SplitType string
type Status string
type Source string

const (
	SplitEqual   SplitType = "equal"
	SplitManual  SplitType = "manual"
	SplitItem    SplitType = "item"
	SplitPercent SplitType = "percent"
	SplitShares  SplitType = "shares"

	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"

	SourceManual  Source = "manual"
	SourceReceipt Source = "receipt"
)

type Payer struct {
	ID, ExpenseID, UserID uuid.UUID
	Amount                decimal.Decimal
	User                  *domainuser.User
	CreatedAt             time.Time
}

type Split struct {
	ID, ExpenseID, UserID uuid.UUID
	Amount                decimal.Decimal
	// Weight carries the caller's original percent value (0-100) or share count for a percent/shares
	// split, alongside the computed Amount, so the client can re-render/edit without losing precision.
	// It is nil for equal/manual/item splits.
	Weight    *decimal.Decimal
	User      *domainuser.User
	CreatedAt time.Time
}

type Expense struct {
	ID, TripID, CreatedByUserID uuid.UUID
	CategoryID                  *uuid.UUID
	ExpenseDate                 time.Time
	Description                 string
	Amount                      decimal.Decimal
	Currency                    string
	// ChargedAmount and ChargedCurrency record what the payer's card or account actually showed,
	// when that differs from Currency (the trip's base currency amount used for splitting). Both
	// are nil unless the expense has one; ChargedAmount is only ever set alongside ChargedCurrency.
	ChargedAmount        *decimal.Decimal
	ChargedCurrency      *string
	SplitType            SplitType
	Status               Status
	Source               Source
	Note                 *string
	ApprovedByUserID     *uuid.UUID
	ApprovedAt           *time.Time
	RejectedReason       *string
	Version              int
	Payers               []Payer
	Splits               []Split
	CreatedBy            *domainuser.User
	ApprovedBy           *domainuser.User
	CreatedAt, UpdatedAt time.Time
}
