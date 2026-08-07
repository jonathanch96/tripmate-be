package balance

import (
	"github.com/google/uuid"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
)

type ParticipantBalance struct {
	UserID                           uuid.UUID
	User                             domainuser.PublicUser
	TotalPaid, TotalOwed, NetBalance decimal.Decimal
}

type Transfer struct {
	FromUserID, ToUserID uuid.UUID
	Amount               decimal.Decimal
	Currency             string
}

type Summary struct {
	TotalExpenses, TotalSettled                 decimal.Decimal
	ExpenseCount, SettlementCount               int
	PendingExpenseCount, PendingSettlementCount int
	UnsettledTotal                              decimal.Decimal
}

type AppliedRate struct {
	From, To string
	Rate     decimal.Decimal
	IsFinal  bool
}

type Result struct {
	BaseCurrency string
	Balances     []ParticipantBalance
	Debts        []Transfer
	Summary      Summary
	RatesUsed    []AppliedRate
}

type FinalPlan struct {
	BaseCurrency string
	Transfers    []Transfer
}
