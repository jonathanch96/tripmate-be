package balanceresponse

import (
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	userresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/user"
)

type User = userresponse.User

type ParticipantBalance struct {
	UserID     uuid.UUID `json:"user_id"`
	User       User      `json:"user"`
	TotalPaid  string    `json:"total_paid"`
	TotalOwed  string    `json:"total_owed"`
	NetBalance string    `json:"net_balance"`
}

type Transfer struct {
	FromUserID uuid.UUID `json:"from_user_id"`
	ToUserID   uuid.UUID `json:"to_user_id"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	// Bank details are present only for viewers allowed to see them — the payer of this transfer
	// or the planner. They are nil for everyone else.
	BankName          *string `json:"bank_name"`
	BankAccountNumber *string `json:"bank_account_number"`
	BankAccountHolder *string `json:"bank_account_holder"`
}

type Summary struct {
	TotalExpenses          string `json:"total_expenses"`
	TotalSettled           string `json:"total_settled"`
	ExpenseCount           int    `json:"expense_count"`
	SettlementCount        int    `json:"settlement_count"`
	PendingExpenseCount    int    `json:"pending_expense_count"`
	PendingSettlementCount int    `json:"pending_settlement_count"`
	UnsettledTotal         string `json:"unsettled_total"`
}

type AppliedRate struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Rate    string `json:"rate"`
	IsFinal bool   `json:"is_final"`
}

type Result struct {
	BaseCurrency string               `json:"base_currency"`
	Balances     []ParticipantBalance `json:"balances"`
	Debts        []Transfer           `json:"debts"`
	Summary      Summary              `json:"summary"`
	RatesUsed    []AppliedRate        `json:"rates_used"`
}

type FinalPlan struct {
	BaseCurrency string     `json:"base_currency"`
	Transfers    []Transfer `json:"transfers"`
}

func FromDomain(entity domainbalance.Result) Result {
	scale := money.DisplayScale(entity.BaseCurrency)
	result := Result{BaseCurrency: entity.BaseCurrency, Debts: TransfersFromDomain(entity.Debts)}
	result.Balances = make([]ParticipantBalance, len(entity.Balances))
	for i, row := range entity.Balances {
		result.Balances[i] = ParticipantBalance{UserID: row.UserID, User: publicUser(row.User),
			TotalPaid: row.TotalPaid.StringFixedBank(scale), TotalOwed: row.TotalOwed.StringFixedBank(scale),
			NetBalance: row.NetBalance.StringFixedBank(scale)}
	}
	result.Summary = Summary{TotalExpenses: entity.Summary.TotalExpenses.StringFixedBank(scale),
		TotalSettled: entity.Summary.TotalSettled.StringFixedBank(scale), ExpenseCount: entity.Summary.ExpenseCount,
		SettlementCount: entity.Summary.SettlementCount, PendingExpenseCount: entity.Summary.PendingExpenseCount,
		PendingSettlementCount: entity.Summary.PendingSettlementCount,
		UnsettledTotal:         entity.Summary.UnsettledTotal.StringFixedBank(scale)}
	result.RatesUsed = make([]AppliedRate, len(entity.RatesUsed))
	for i, rate := range entity.RatesUsed {
		result.RatesUsed[i] = AppliedRate{From: rate.From, To: rate.To, Rate: rate.Rate.StringFixed(12), IsFinal: rate.IsFinal}
	}
	return result
}

func FinalPlanFromDomain(entity domainbalance.FinalPlan) FinalPlan {
	return FinalPlan{BaseCurrency: entity.BaseCurrency, Transfers: TransfersFromDomain(entity.Transfers)}
}

type LedgerEntry struct {
	Kind               string     `json:"kind"`
	Date               string     `json:"date"`
	ExpenseID          *uuid.UUID `json:"expense_id,omitempty"`
	SettlementID       *uuid.UUID `json:"settlement_id,omitempty"`
	Description        string     `json:"description"`
	CategoryID         *uuid.UUID `json:"category_id,omitempty"`
	// Paid/Share are only present for expense rows in the unfiltered ("against everyone") view.
	Paid               *string    `json:"paid,omitempty"`
	Share              *string    `json:"share,omitempty"`
	CounterpartyUserID *uuid.UUID `json:"counterparty_user_id,omitempty"`
	Delta              string     `json:"delta"`
	RunningBalance     string     `json:"running_balance"`
}

type Ledger struct {
	BaseCurrency  string        `json:"base_currency"`
	MemberUserID  uuid.UUID     `json:"member_user_id"`
	AgainstUserID *uuid.UUID    `json:"against_user_id,omitempty"`
	Entries       []LedgerEntry `json:"entries"`
	NetBalance    string        `json:"net_balance"`
}

func LedgerFromDomain(entity domainbalance.Ledger) Ledger {
	scale := money.DisplayScale(entity.BaseCurrency)
	entries := make([]LedgerEntry, len(entity.Entries))
	for i, row := range entity.Entries {
		entries[i] = LedgerEntry{Kind: string(row.Kind), Date: row.Date.Format("2006-01-02"), ExpenseID: row.ExpenseID,
			SettlementID: row.SettlementID, Description: row.Description, CategoryID: row.CategoryID,
			CounterpartyUserID: row.CounterpartyUserID, Delta: row.Delta.StringFixedBank(scale), RunningBalance: row.RunningBalance.StringFixedBank(scale)}
		if row.Paid != nil {
			paid := row.Paid.StringFixedBank(scale)
			entries[i].Paid = &paid
		}
		if row.Share != nil {
			share := row.Share.StringFixedBank(scale)
			entries[i].Share = &share
		}
	}
	return Ledger{BaseCurrency: entity.BaseCurrency, MemberUserID: entity.MemberUserID, AgainstUserID: entity.AgainstUserID,
		Entries: entries, NetBalance: entity.NetBalance.StringFixedBank(scale)}
}

func TransfersFromDomain(entities []domainbalance.Transfer) []Transfer {
	rows := make([]Transfer, len(entities))
	for i, row := range entities {
		rows[i] = Transfer{FromUserID: row.FromUserID, ToUserID: row.ToUserID,
			Amount: row.Amount.StringFixedBank(money.DisplayScale(row.Currency)), Currency: row.Currency,
			BankName: row.BankName, BankAccountNumber: row.BankAccountNumber, BankAccountHolder: row.BankAccountHolder}
	}
	return rows
}

func publicUser(entity domainuser.PublicUser) User {
	return User{ID: entity.ID, Email: entity.Email, Name: entity.Name, AvatarURL: entity.AvatarURL,
		CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
