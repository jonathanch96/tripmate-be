package balance

import (
	"time"

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
	BankName             *string
	BankAccountNumber    *string
	BankAccountHolder    *string
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

type LedgerEntryKind string

const (
	LedgerEntryExpense    LedgerEntryKind = "expense"
	LedgerEntrySettlement LedgerEntryKind = "settlement"
)

// LedgerEntry is one line of a single member's running statement - either their net position on
// an expense (how much they paid minus their own share) or a settlement they sent/received. Paid
// and Share are only populated for expense rows in the unfiltered ("against everyone") view, where
// the UI shows "Paid X - share Y"; a pairwise ("against" a specific member) view collapses an
// expense to a single Delta between the two people instead.
type LedgerEntry struct {
	Kind               LedgerEntryKind
	Date               time.Time
	ExpenseID          *uuid.UUID
	SettlementID       *uuid.UUID
	Description        string
	CategoryID         *uuid.UUID
	Paid               *decimal.Decimal
	Share              *decimal.Decimal
	CounterpartyUserID *uuid.UUID
	Delta              decimal.Decimal
	RunningBalance     decimal.Decimal
}

// Ledger is one member's statement of every expense/settlement that actually moved their balance -
// entries where a member paid exactly their own share (nothing owed either way) are left out
// entirely, matching how a bank statement only lists transactions, not non-events. When
// AgainstUserID is set, every entry is collapsed to the net effect between just those two members.
type Ledger struct {
	BaseCurrency  string
	MemberUserID  uuid.UUID
	AgainstUserID *uuid.UUID
	Entries       []LedgerEntry
	NetBalance    decimal.Decimal
}
