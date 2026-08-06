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
	SplitEqual  SplitType = "equal"
	SplitManual SplitType = "manual"
	SplitItem   SplitType = "item"

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
	User                  *domainuser.User
	CreatedAt             time.Time
}

type Expense struct {
	ID, TripID, CreatedByUserID uuid.UUID
	ExpenseDate                 time.Time
	Description                 string
	Amount                      decimal.Decimal
	Currency                    string
	SplitType                   SplitType
	Status                      Status
	Source                      Source
	Note                        *string
	ApprovedByUserID            *uuid.UUID
	ApprovedAt                  *time.Time
	RejectedReason              *string
	Version                     int
	Payers                      []Payer
	Splits                      []Split
	CreatedBy                   *domainuser.User
	ApprovedBy                  *domainuser.User
	CreatedAt, UpdatedAt        time.Time
}
