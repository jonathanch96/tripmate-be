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
	SplitType    domainexpense.SplitType
	Payers       []domainexpense.Payer
	Participants []uuid.UUID
	Manual       map[uuid.UUID]decimal.Decimal
	Note         *string
}

type UpdateInput struct {
	ExpenseDate  *time.Time
	Description  *string
	Amount       *decimal.Decimal
	Currency     *string
	SplitType    *domainexpense.SplitType
	Payers       []domainexpense.Payer
	Participants []uuid.UUID
	Manual       map[uuid.UUID]decimal.Decimal
	Note         *string
	Version      int
}

type Totals struct {
	ByCurrency    map[string]decimal.Decimal
	CountByStatus map[domainexpense.Status]int64
}
