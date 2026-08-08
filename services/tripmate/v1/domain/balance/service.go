package balance

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"strings"

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
		converted, convertErr := table.Convert(expense.Amount, expense.Currency, tc.Trip.BaseCurrency)
		if convertErr != nil {
			return nil, convertErr
		}
		summary.TotalExpenses = summary.TotalExpenses.Add(converted)
		rememberRate(used, table, expense.Currency, tc.Trip.BaseCurrency)
		for _, payer := range expense.Payers {
			amount, convertErr := table.Convert(payer.Amount, expense.Currency, tc.Trip.BaseCurrency)
			if convertErr != nil {
				return nil, convertErr
			}
			row := byUser[payer.UserID]
			row.paid = row.paid.Add(amount)
			byUser[payer.UserID] = row
		}
		for _, split := range expense.Splits {
			amount, convertErr := table.Convert(split.Amount, expense.Currency, tc.Trip.BaseCurrency)
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

func validateRates(table *fxdomain.RateTable, trip domaintrip.Trip, expenses []domainexpense.Expense, settlements []domainsettlement.Settlement) error {
	pairs := make(map[string]struct{})
	for _, expense := range expenses {
		if expense.Status == domainexpense.StatusApproved && !strings.EqualFold(expense.Currency, trip.BaseCurrency) {
			pairs[strings.ToUpper(expense.Currency)+"→"+strings.ToUpper(trip.BaseCurrency)] = struct{}{}
		}
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
