package balance

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
)

// ledgerEpsilon mirrors the money package's rounding tolerance: a delta this small is rounding
// noise, not a real transaction, and is left out of a member's statement entirely.
var ledgerEpsilon = decimal.NewFromFloat(0.005)

type service struct{ deps Dependencies }

type totals struct{ paid, owed decimal.Decimal }

func NewService(deps Dependencies) Service { return &service{deps: deps} }

func (s *service) Calculate(ctx context.Context, tc tripctx.TripContext) (*domainbalance.Result, error) {
	participants, err := s.deps.Participants.ListByTripID(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	expenses, err := s.deps.Expenses.ListForBalance(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	settlements, err := s.deps.Settlements.ListForBalance(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	table, err := s.deps.FX.EffectiveTable(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	if err = validateRates(table, tc.Trip, expenses, settlements); err != nil {
		return nil, err
	}

	byUser := make(map[uuid.UUID]totals, len(participants))
	users := make(map[uuid.UUID]domainuser.PublicUser, len(participants))
	for _, participant := range participants {
		byUser[participant.UserID] = totals{}
		if participant.User != nil {
			users[participant.UserID] = *participant.User
		} else {
			users[participant.UserID] = domainuser.PublicUser{ID: participant.UserID}
		}
	}

	summary := domainbalance.Summary{}
	used := make(map[string]domainbalance.AppliedRate)
	for _, expense := range expenses {
		if expense.Status == domainexpense.StatusPending {
			summary.PendingExpenseCount++
		}
		if expense.Status != domainexpense.StatusApproved {
			continue
		}
		summary.ExpenseCount++
		convert, hasOverride := expenseConverter(expense, table, tc.Trip.BaseCurrency)
		converted, convertErr := convert(expense.Amount)
		if convertErr != nil {
			return nil, convertErr
		}
		if hasOverride {
			// Use the exact entered total rather than the re-derived (amount * overrideRate) figure,
			// which can drift from it by a rounding fraction at the 12-decimal-place rate precision.
			converted = *expense.ChargedAmount
		} else {
			rememberRate(used, table, expense.Currency, tc.Trip.BaseCurrency)
		}
		summary.TotalExpenses = summary.TotalExpenses.Add(converted)
		for _, payer := range expense.Payers {
			amount, convertErr := convert(payer.Amount)
			if convertErr != nil {
				return nil, convertErr
			}
			row := byUser[payer.UserID]
			row.paid = row.paid.Add(amount)
			byUser[payer.UserID] = row
		}
		for _, split := range expense.Splits {
			amount, convertErr := convert(split.Amount)
			if convertErr != nil {
				return nil, convertErr
			}
			row := byUser[split.UserID]
			row.owed = row.owed.Add(amount)
			byUser[split.UserID] = row
		}
	}
	for _, settlement := range settlements {
		if settlement.Status == domainsettlement.StatusPending {
			summary.PendingSettlementCount++
		}
		if settlement.Status != domainsettlement.StatusApproved {
			continue
		}
		amount, convertErr := table.Convert(settlement.Amount, settlement.Currency, tc.Trip.BaseCurrency)
		if convertErr != nil {
			return nil, convertErr
		}
		summary.SettlementCount++
		summary.TotalSettled = summary.TotalSettled.Add(amount)
		from := byUser[settlement.FromUserID]
		from.paid = from.paid.Add(amount)
		byUser[settlement.FromUserID] = from
		to := byUser[settlement.ToUserID]
		to.owed = to.owed.Add(amount)
		byUser[settlement.ToUserID] = to
		rememberRate(used, table, settlement.Currency, tc.Trip.BaseCurrency)
	}

	result := &domainbalance.Result{BaseCurrency: strings.ToUpper(tc.Trip.BaseCurrency)}
	result.Balances = make([]domainbalance.ParticipantBalance, 0, len(participants))
	for userID, user := range users {
		row := byUser[userID]
		result.Balances = append(result.Balances, domainbalance.ParticipantBalance{UserID: userID, User: user, TotalPaid: row.paid, TotalOwed: row.owed, NetBalance: row.paid.Sub(row.owed)})
	}
	sort.Slice(result.Balances, func(i, j int) bool {
		return bytes.Compare(result.Balances[i].UserID[:], result.Balances[j].UserID[:]) < 0
	})
	result.Debts = Optimize(result.Balances, result.BaseCurrency)
	netTotal := decimal.Zero
	for _, row := range result.Balances {
		netTotal = netTotal.Add(row.NetBalance)
	}
	if !netTotal.IsZero() {
		slog.Error("participant balances do not sum to zero", "trip_id", tc.Trip.ID, "net_total", netTotal.String())
	}
	for _, debt := range result.Debts {
		summary.UnsettledTotal = summary.UnsettledTotal.Add(debt.Amount)
	}
	result.Summary = summary
	result.RatesUsed = make([]domainbalance.AppliedRate, 0, len(used))
	for _, rate := range used {
		result.RatesUsed = append(result.RatesUsed, rate)
	}
	sort.Slice(result.RatesUsed, func(i, j int) bool {
		return result.RatesUsed[i].From+result.RatesUsed[i].To < result.RatesUsed[j].From+result.RatesUsed[j].To
	})
	return result, nil
}

func (s *service) Summary(ctx context.Context, tc tripctx.TripContext) (*domainbalance.Summary, error) {
	result, err := s.Calculate(ctx, tc)
	if err != nil {
		return nil, err
	}
	return &result.Summary, nil
}

func (s *service) OutstandingDebt(ctx context.Context, tripID, fromID, toID uuid.UUID) (decimal.Decimal, error) {
	if s.deps.Trips == nil {
		return decimal.Zero, apperror.New("INTERNAL_ERROR")
	}
	trip, err := s.deps.Trips.GetByID(ctx, tripID)
	if err != nil {
		return decimal.Zero, err
	}
	result, err := s.Calculate(ctx, tripctx.TripContext{Trip: *trip})
	if err != nil {
		return decimal.Zero, err
	}
	// A bilateral cap, not a lookup in the optimizer's suggested transfer list. The optimizer emits
	// *a* minimum-cardinality set of transfers that zeroes everyone out, so a legitimate payment
	// between two people it happened not to pair would find no edge and be rejected outright.
	// What actually bounds a payment from -> to is how much `from` is short overall and how much
	// `to` is still owed overall; either side hitting zero means the payment is not settling a debt.
	var owedByFrom, owedToTo decimal.Decimal
	for _, row := range result.Balances {
		switch row.UserID {
		case fromID:
			owedByFrom = row.NetBalance.Neg()
		case toID:
			owedToTo = row.NetBalance
		}
	}
	if owedByFrom.IsNegative() || owedToTo.IsNegative() {
		return decimal.Zero, nil
	}
	return decimal.Min(owedByFrom, owedToTo), nil
}

// Ledger builds one member's statement: every expense/settlement that actually moved their
// balance, oldest first, with a running total - like a bank statement. An expense where the
// member paid exactly their own share (or wasn't involved at all) nets to zero and is left out
// entirely, same as a bank statement only lists real transactions.
//
// When filter.AgainstUserID is set, every entry collapses to the net effect between just those two
// members: for an expense, each split-member's share is distributed across payers in proportion to
// what they paid, a payer's own share of what they themselves paid is skipped (not a transaction
// between two people), and only the portions that cross between the member and the counterparty are
// kept.
func (s *service) Ledger(ctx context.Context, tc tripctx.TripContext, filter LedgerFilter) (*domainbalance.Ledger, error) {
	expenses, err := s.deps.Expenses.ListForBalance(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	settlements, err := s.deps.Settlements.ListForBalance(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	table, err := s.deps.FX.EffectiveTable(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	if err = validateRates(table, tc.Trip, expenses, settlements); err != nil {
		return nil, err
	}

	member := filter.MemberUserID
	filtering := filter.AgainstUserID != nil && *filter.AgainstUserID != member
	var against uuid.UUID
	if filtering {
		against = *filter.AgainstUserID
	}

	type draft struct {
		kind               domainbalance.LedgerEntryKind
		date               time.Time
		expenseID          *uuid.UUID
		settlementID       *uuid.UUID
		description        string
		categoryID         *uuid.UUID
		paid, share        *decimal.Decimal
		counterpartyUserID *uuid.UUID
		delta              decimal.Decimal
	}
	var drafts []draft

	for _, expense := range expenses {
		if expense.Status != domainexpense.StatusApproved {
			continue
		}
		convert, _ := expenseConverter(expense, table, tc.Trip.BaseCurrency)
		id := expense.ID
		if !filtering {
			paid := decimal.Zero
			for _, payer := range expense.Payers {
				if payer.UserID != member {
					continue
				}
				amount, convertErr := convert(payer.Amount)
				if convertErr != nil {
					return nil, convertErr
				}
				paid = paid.Add(amount)
			}
			share := decimal.Zero
			for _, split := range expense.Splits {
				if split.UserID != member {
					continue
				}
				amount, convertErr := convert(split.Amount)
				if convertErr != nil {
					return nil, convertErr
				}
				share = share.Add(amount)
			}
			if delta := paid.Sub(share); delta.Abs().GreaterThan(ledgerEpsilon) {
				drafts = append(drafts, draft{kind: domainbalance.LedgerEntryExpense, date: expense.ExpenseDate, expenseID: &id,
					description: expense.Description, categoryID: expense.CategoryID, paid: &paid, share: &share, delta: delta})
			}
			continue
		}
		totalPaid := decimal.Zero
		for _, payer := range expense.Payers {
			amount, convertErr := convert(payer.Amount)
			if convertErr != nil {
				return nil, convertErr
			}
			totalPaid = totalPaid.Add(amount)
		}
		if !totalPaid.IsPositive() {
			continue
		}
		delta := decimal.Zero
		for _, payer := range expense.Payers {
			paidByPayer, convertErr := convert(payer.Amount)
			if convertErr != nil {
				return nil, convertErr
			}
			for _, split := range expense.Splits {
				if split.UserID == payer.UserID {
					continue
				}
				splitAmount, convertErr := convert(split.Amount)
				if convertErr != nil {
					return nil, convertErr
				}
				debt := splitAmount.Mul(paidByPayer).Div(totalPaid)
				switch {
				case split.UserID == member && payer.UserID == against:
					delta = delta.Sub(debt)
				case split.UserID == against && payer.UserID == member:
					delta = delta.Add(debt)
				}
			}
		}
		if delta.Abs().GreaterThan(ledgerEpsilon) {
			counterparty := against
			drafts = append(drafts, draft{kind: domainbalance.LedgerEntryExpense, date: expense.ExpenseDate, expenseID: &id,
				description: expense.Description, categoryID: expense.CategoryID, counterpartyUserID: &counterparty, delta: delta})
		}
	}

	for _, settlement := range settlements {
		if settlement.Status != domainsettlement.StatusApproved {
			continue
		}
		amount, convertErr := table.Convert(settlement.Amount, settlement.Currency, tc.Trip.BaseCurrency)
		if convertErr != nil {
			return nil, convertErr
		}
		id := settlement.ID
		from, to := settlement.FromUserID, settlement.ToUserID
		sent, received := from == member && (!filtering || to == against), to == member && (!filtering || from == against)
		switch {
		case sent:
			counterparty := to
			drafts = append(drafts, draft{kind: domainbalance.LedgerEntrySettlement, date: settlement.CreatedAt, settlementID: &id,
				description: "Settlement", counterpartyUserID: &counterparty, delta: amount})
		case received:
			counterparty := from
			drafts = append(drafts, draft{kind: domainbalance.LedgerEntrySettlement, date: settlement.CreatedAt, settlementID: &id,
				description: "Settlement", counterpartyUserID: &counterparty, delta: amount.Neg()})
		}
	}

	sort.SliceStable(drafts, func(i, j int) bool { return drafts[i].date.Before(drafts[j].date) })
	entries := make([]domainbalance.LedgerEntry, len(drafts))
	running := decimal.Zero
	for i, d := range drafts {
		running = running.Add(d.delta)
		entries[i] = domainbalance.LedgerEntry{Kind: d.kind, Date: d.date, ExpenseID: d.expenseID, SettlementID: d.settlementID,
			Description: d.description, CategoryID: d.categoryID, Paid: d.paid, Share: d.share,
			CounterpartyUserID: d.counterpartyUserID, Delta: d.delta, RunningBalance: running}
	}
	result := &domainbalance.Ledger{BaseCurrency: strings.ToUpper(tc.Trip.BaseCurrency), MemberUserID: member, Entries: entries, NetBalance: running}
	if filtering {
		result.AgainstUserID = &against
	}
	return result, nil
}

func (s *service) FinalSettlement(ctx context.Context, tc tripctx.TripContext) (*domainbalance.FinalPlan, error) {
	result, err := s.Calculate(ctx, tc)
	if err != nil {
		return nil, err
	}
	participants, err := s.deps.Participants.ListByTripID(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	bankByUser := make(map[uuid.UUID]*domainparticipant.BankInfo, len(participants))
	for _, participant := range participants {
		bankByUser[participant.UserID] = participant.BankInfo
	}
	transfers := append([]domainbalance.Transfer(nil), result.Debts...)
	for i := range transfers {
		// Full bank details are restricted to the payer and planner on this view.
		if tc.Participant.Role != domainparticipant.RolePlanner && tc.Participant.UserID != transfers[i].FromUserID {
			continue
		}
		bank := bankByUser[transfers[i].ToUserID]
		if bank == nil {
			continue
		}
		transfers[i].BankName = &bank.BankName
		transfers[i].BankAccountNumber = &bank.AccountNumber
		transfers[i].BankAccountHolder = &bank.AccountHolder
	}
	return &domainbalance.FinalPlan{BaseCurrency: result.BaseCurrency, Transfers: transfers}, nil
}

// expenseConverter returns a function that converts an amount in this expense's own currency into
// the trip's base currency - using the expense's own charged-amount override when it has one,
// falling back to the trip's saved rate table otherwise - plus whether an override was used.
func expenseConverter(expense domainexpense.Expense, table *fxdomain.RateTable, tripBaseCurrency string) (func(decimal.Decimal) (decimal.Decimal, error), bool) {
	overrideRate, hasOverride := chargedOverrideRate(expense, tripBaseCurrency)
	return func(amount decimal.Decimal) (decimal.Decimal, error) {
		if hasOverride {
			return amount.Mul(overrideRate), nil
		}
		return table.Convert(amount, expense.Currency, tripBaseCurrency)
	}, hasOverride
}

// chargedOverrideRate returns the per-transaction conversion rate for an expense carrying an
// authoritative base-currency charged amount, and whether one applies. A charged currency other
// than the trip's base is never persisted (enforced at write time), so this only needs to guard
// against a nil/zero amount.
func chargedOverrideRate(expense domainexpense.Expense, tripBaseCurrency string) (decimal.Decimal, bool) {
	if expense.ChargedAmount == nil || expense.ChargedCurrency == nil {
		return decimal.Zero, false
	}
	if !strings.EqualFold(*expense.ChargedCurrency, tripBaseCurrency) || expense.Amount.IsZero() {
		return decimal.Zero, false
	}
	return expense.ChargedAmount.DivRound(expense.Amount, 12), true
}

func validateRates(table *fxdomain.RateTable, trip domaintrip.Trip, expenses []domainexpense.Expense, settlements []domainsettlement.Settlement) error {
	pairs := make(map[string]struct{})
	for _, expense := range expenses {
		if expense.Status != domainexpense.StatusApproved || strings.EqualFold(expense.Currency, trip.BaseCurrency) {
			continue
		}
		if _, hasOverride := chargedOverrideRate(expense, trip.BaseCurrency); hasOverride {
			// This expense carries its own per-transaction rate, so no trip-level rate is required.
			continue
		}
		pairs[strings.ToUpper(expense.Currency)+"→"+strings.ToUpper(trip.BaseCurrency)] = struct{}{}
	}
	for _, settlement := range settlements {
		if settlement.Status == domainsettlement.StatusApproved && !strings.EqualFold(settlement.Currency, trip.BaseCurrency) {
			pairs[strings.ToUpper(settlement.Currency)+"→"+strings.ToUpper(trip.BaseCurrency)] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for pair := range pairs {
		parts := strings.Split(pair, "→")
		if _, err := table.Convert(decimal.NewFromInt(1), parts[0], parts[1]); err != nil {
			missing = append(missing, pair)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return apperror.Newf("EXCHANGE_RATE_MISSING", "Exchange rates required: %s", strings.Join(missing, ", "))
}

func rememberRate(used map[string]domainbalance.AppliedRate, table *fxdomain.RateTable, from, to string) {
	rate, ok := table.Lookup(from, to)
	if !ok {
		return
	}
	key := rate.FromCurrency + "\x00" + rate.ToCurrency
	used[key] = domainbalance.AppliedRate{From: rate.FromCurrency, To: rate.ToCurrency, Rate: rate.Rate, IsFinal: rate.IsFinal}
}
