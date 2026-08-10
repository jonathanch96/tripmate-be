package expense

import (
	"time"

	"github.com/google/uuid"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"github.com/shopspring/decimal"
)

type Filter struct {
	PayerUserID *uuid.UUID
	SplitUserID *uuid.UUID
	Status      *domainexpense.Status
	Currency    string
	DateFrom    *time.Time
	DateTo      *time.Time
	Query       string
	Sort        string
	Page        int
	PerPage     int
}

type CreateInput struct {
	ExpenseDate  time.Time
	Description  string
	Amount       decimal.Decimal
	Currency     string
	CategoryID   *uuid.UUID
	SplitType    domainexpense.SplitType
	Payers       []domainexpense.Payer
	Participants []uuid.UUID
	Manual       map[uuid.UUID]decimal.Decimal
	// Weights carries percentages/shares for a percent/shares split. Only read for those split types.
	Weights map[uuid.UUID]decimal.Decimal
	Note    *string
	// Items and Extras carry an itemised bill: which lines each diner shared, and the tax and
	// service to spread across them. Only read for SplitItem.
	Items  []ItemAssignment
	Extras decimal.Decimal
	// Source records where the expense came from. Empty means a person typed it in.
	Source domainexpense.Source
}

type UpdateInput struct {
	ExpenseDate  *time.Time
	Description  *string
	Amount       *decimal.Decimal
	Currency     *string
	CategoryID   *uuid.UUID
	SplitType    *domainexpense.SplitType
	Payers       []domainexpense.Payer
	Participants []uuid.UUID
	Manual       map[uuid.UUID]decimal.Decimal
	Weights      map[uuid.UUID]decimal.Decimal
	Note         *string
	Version      int
}

type Totals struct {
	ByCurrency    map[string]decimal.Decimal
	CountByStatus map[domainexpense.Status]int64
}
