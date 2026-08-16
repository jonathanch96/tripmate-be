package balance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

type expenseRows []domainexpense.Expense

func (rows expenseRows) ListForBalance(context.Context, uuid.UUID) ([]domainexpense.Expense, error) {
	return rows, nil
}

type settlementRows []domainsettlement.Settlement

func (rows settlementRows) ListForBalance(context.Context, uuid.UUID) ([]domainsettlement.Settlement, error) {
	return rows, nil
}

type participantRows []domainparticipant.Participant

func (rows participantRows) ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error) {
	return rows, nil
}

type tableProvider struct{ table *fxdomain.RateTable }

func (provider tableProvider) EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error) {
	return provider.table, nil
}

func participant(id uuid.UUID, name string) domainparticipant.Participant {
	return domainparticipant.Participant{UserID: id, User: &domainuser.PublicUser{ID: id, Name: name, Email: strings.ToLower(name) + "@example.com"}}
}

func TestCalculateFiveHandComputedFixtures(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	c := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	participants := []domainparticipant.Participant{participant(c, "Cara"), participant(a, "Ana"), participant(b, "Ben")}
	baseExpense := domainexpense.Expense{Amount: decimal.NewFromInt(120), Currency: "PHP", Status: domainexpense.StatusApproved, Payers: []domainexpense.Payer{{UserID: a, Amount: decimal.NewFromInt(80)}, {UserID: b, Amount: decimal.NewFromInt(40)}}, Splits: []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(40)}, {UserID: b, Amount: decimal.NewFromInt(40)}, {UserID: c, Amount: decimal.NewFromInt(40)}}}

	tests := []struct {
		name                                                                               string
		expenses                                                                           expenseRows
		settlements                                                                        settlementRows
		rates                                                                              []domainfx.Rate
		wantNet                                                                            map[uuid.UUID]string
		wantExpense, wantSettled, wantUnsettled                                            string
		wantExpenseCount, wantSettlementCount, wantPendingExpenses, wantPendingSettlements int
	}{
		{name: "multi-payer base currency", expenses: expenseRows{baseExpense}, wantNet: map[uuid.UUID]string{a: "40", b: "0", c: "-40"}, wantExpense: "120", wantSettled: "0", wantUnsettled: "40", wantExpenseCount: 1},
		{name: "approved settlement reduces debt", expenses: expenseRows{baseExpense}, settlements: settlementRows{{FromUserID: c, ToUserID: a, Amount: decimal.NewFromInt(10), Currency: "PHP", Status: domainsettlement.StatusApproved}}, wantNet: map[uuid.UUID]string{a: "30", b: "0", c: "-30"}, wantExpense: "120", wantSettled: "10", wantUnsettled: "30", wantExpenseCount: 1, wantSettlementCount: 1},
		{name: "foreign expense uses one effective rate", expenses: expenseRows{{Amount: decimal.NewFromInt(2), Currency: "USD", Status: domainexpense.StatusApproved, Payers: []domainexpense.Payer{{UserID: a, Amount: decimal.NewFromInt(2)}}, Splits: []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(1)}, {UserID: b, Amount: decimal.NewFromInt(1)}}}}, rates: []domainfx.Rate{{FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.NewFromInt(50)}}, wantNet: map[uuid.UUID]string{a: "50", b: "-50", c: "0"}, wantExpense: "100", wantSettled: "0", wantUnsettled: "50", wantExpenseCount: 1},
		{name: "pending activity is excluded", expenses: expenseRows{baseExpense, domainexpense.Expense{Amount: decimal.NewFromInt(999), Currency: "PHP", Status: domainexpense.StatusPending}}, settlements: settlementRows{{FromUserID: c, ToUserID: a, Amount: decimal.NewFromInt(999), Currency: "PHP", Status: domainsettlement.StatusPending}}, wantNet: map[uuid.UUID]string{a: "40", b: "0", c: "-40"}, wantExpense: "120", wantSettled: "0", wantUnsettled: "40", wantExpenseCount: 1, wantPendingExpenses: 1, wantPendingSettlements: 1},
		{name: "zero-activity participants remain visible", wantNet: map[uuid.UUID]string{a: "0", b: "0", c: "0"}, wantExpense: "0", wantSettled: "0", wantUnsettled: "0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := balancedomain.NewService(balancedomain.Dependencies{Expenses: test.expenses, Settlements: test.settlements, Participants: participantRows(participants), FX: tableProvider{table: fxdomain.NewRateTable(test.rates)}})
			result, err := service.Calculate(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Balances) != 3 || result.Balances[0].UserID != a || result.Balances[1].UserID != b || result.Balances[2].UserID != c {
				t.Fatalf("balances are not complete and UUID-sorted: %+v", result.Balances)
			}
			for _, row := range result.Balances {
				if !row.NetBalance.Equal(decimal.RequireFromString(test.wantNet[row.UserID])) {
					t.Fatalf("net[%s] = %s, want %s", row.UserID, row.NetBalance, test.wantNet[row.UserID])
				}
			}
			if !result.Summary.TotalExpenses.Equal(decimal.RequireFromString(test.wantExpense)) || !result.Summary.TotalSettled.Equal(decimal.RequireFromString(test.wantSettled)) || !result.Summary.UnsettledTotal.Equal(decimal.RequireFromString(test.wantUnsettled)) {
				t.Fatalf("summary money = %+v", result.Summary)
			}
			if result.Summary.ExpenseCount != test.wantExpenseCount || result.Summary.SettlementCount != test.wantSettlementCount || result.Summary.PendingExpenseCount != test.wantPendingExpenses || result.Summary.PendingSettlementCount != test.wantPendingSettlements {
				t.Fatalf("summary counts = %+v", result.Summary)
			}
			if test.name == "foreign expense uses one effective rate" {
				if len(result.RatesUsed) != 1 || result.RatesUsed[0].From != "USD" || result.RatesUsed[0].To != "PHP" || !result.RatesUsed[0].Rate.Equal(decimal.NewFromInt(50)) {
					t.Fatalf("rates used = %+v", result.RatesUsed)
				}
			}
		})
	}
}

// chargedExpense builds an approved expense recorded in a foreign currency together with an
// authoritative base-currency charged amount — the per-transaction rate override.
func chargedExpense(currency string, amount int64, payer uuid.UUID, chargedAmount int64, baseCurrency string, splits []domainexpense.Split) domainexpense.Expense {
	charged := decimal.NewFromInt(chargedAmount)
	return domainexpense.Expense{Amount: decimal.NewFromInt(amount), Currency: currency, Status: domainexpense.StatusApproved,
		ChargedAmount: &charged, ChargedCurrency: &baseCurrency,
		Payers: []domainexpense.Payer{{UserID: payer, Amount: decimal.NewFromInt(amount)}}, Splits: splits}
}

func TestCalculateChargedAmountOverridesPerExpenseRate(t *testing.T) {
	a, b := uuid.UUID{15: 1}, uuid.UUID{15: 2}
	participants := []domainparticipant.Participant{participant(a, "Ana"), participant(b, "Ben")}
	// 1500 THB paid by card, actually charged 840000 IDR — a per-transaction rate of 560, split
	// evenly. No THB→IDR trip rate is configured at all, proving the override removes the need for
	// one (validateRates must not demand it).
	expense := chargedExpense("THB", 1500, a, 840000, "IDR", []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(750)}, {UserID: b, Amount: decimal.NewFromInt(750)}})
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{expense}, Settlements: settlementRows{}, Participants: participantRows(participants), FX: tableProvider{table: fxdomain.NewRateTable(nil)}})

	result, err := service.Calculate(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "IDR"}})
	if err != nil {
		t.Fatalf("Calculate() error = %v, want no error even without a configured THB→IDR rate", err)
	}
	if !result.Summary.TotalExpenses.Equal(decimal.NewFromInt(840000)) {
		t.Fatalf("TotalExpenses = %s, want 840000 (the exact charged amount, not a re-derived figure)", result.Summary.TotalExpenses)
	}
	wantNet := map[uuid.UUID]string{a: "420000", b: "-420000"}
	for _, row := range result.Balances {
		if !row.NetBalance.Equal(decimal.RequireFromString(wantNet[row.UserID])) {
			t.Fatalf("net[%s] = %s, want %s", row.UserID, row.NetBalance, wantNet[row.UserID])
		}
	}
	if len(result.RatesUsed) != 0 {
		t.Fatalf("RatesUsed = %+v, want empty — a per-expense override is not a trip-level rate", result.RatesUsed)
	}
}

func TestCalculateChargedAmountOverrideCoexistsWithNormalForeignExpense(t *testing.T) {
	a, b := uuid.UUID{15: 1}, uuid.UUID{15: 2}
	participants := []domainparticipant.Participant{participant(a, "Ana"), participant(b, "Ben")}
	// One THB expense with its own charged-amount override (no configured rate needed for it), plus
	// one plain USD expense that does need — and has — a configured trip rate. Only the pair that's
	// actually required (USD→IDR) should be enforced.
	overridden := chargedExpense("THB", 1500, a, 840000, "IDR", []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(1500)}})
	plain := domainexpense.Expense{Amount: decimal.NewFromInt(10), Currency: "USD", Status: domainexpense.StatusApproved,
		Payers: []domainexpense.Payer{{UserID: b, Amount: decimal.NewFromInt(10)}}, Splits: []domainexpense.Split{{UserID: b, Amount: decimal.NewFromInt(10)}}}
	rates := []domainfx.Rate{{FromCurrency: "USD", ToCurrency: "IDR", Rate: decimal.NewFromInt(15000)}}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{overridden, plain}, Settlements: settlementRows{}, Participants: participantRows(participants), FX: tableProvider{table: fxdomain.NewRateTable(rates)}})

	result, err := service.Calculate(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "IDR"}})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if !result.Summary.TotalExpenses.Equal(decimal.NewFromInt(990000)) {
		t.Fatalf("TotalExpenses = %s, want 990000 (840000 override + 150000 at the configured USD rate)", result.Summary.TotalExpenses)
	}
	if len(result.RatesUsed) != 1 || result.RatesUsed[0].From != "USD" {
		t.Fatalf("RatesUsed = %+v, want only the USD→IDR trip rate", result.RatesUsed)
	}
}

func TestCalculateListsEveryMissingCurrencyPair(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{
		{Amount: decimal.NewFromInt(1), Currency: "USD", Status: domainexpense.StatusApproved},
		{Amount: decimal.NewFromInt(1), Currency: "EUR", Status: domainexpense.StatusApproved},
	}, Settlements: settlementRows{}, Participants: participantRows{participant(a, "Ana"), participant(b, "Ben")}, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})
	_, err := service.Calculate(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}})
	if !apperror.Is(err, "EXCHANGE_RATE_MISSING") || !strings.Contains(err.(*apperror.Error).Message, "EUR→PHP") || !strings.Contains(err.(*apperror.Error).Message, "USD→PHP") {
		t.Fatalf("Calculate() error = %v", err)
	}
}

func TestCalculateAlwaysBalancesToZero(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a, b := uuid.UUID{15: 1}, uuid.UUID{15: 2}
		amount := decimal.NewFromInt(rapid.Int64Range(1, 1_000_000).Draw(t, "amount"))
		owedByA := decimal.NewFromInt(rapid.Int64Range(0, amount.IntPart()).Draw(t, "owed_by_a"))
		service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{{Amount: amount, Currency: "PHP", Status: domainexpense.StatusApproved, Payers: []domainexpense.Payer{{UserID: a, Amount: amount}}, Splits: []domainexpense.Split{{UserID: a, Amount: owedByA}, {UserID: b, Amount: amount.Sub(owedByA)}}}}, Settlements: settlementRows{}, Participants: participantRows{participant(a, "Ana"), participant(b, "Ben")}, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})
		result, err := service.Calculate(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}})
		if err != nil {
			t.Fatal(err)
		}
		total := decimal.Zero
		for _, row := range result.Balances {
			total = total.Add(row.NetBalance)
		}
		if !total.IsZero() {
			t.Fatalf("sum(net) = %s", total)
		}
	})
}

type tripRow struct{ trip domaintrip.Trip }

func (row tripRow) GetByID(context.Context, uuid.UUID) (*domaintrip.Trip, error) {
	return &row.trip, nil
}

// paidFor is one approved expense where payer covers the whole amount and debtor owes all of it.
func paidFor(payer, debtor uuid.UUID, amount int64, currency string) domainexpense.Expense {
	value := decimal.NewFromInt(amount)
	return domainexpense.Expense{Amount: value, Currency: currency, Status: domainexpense.StatusApproved,
		Payers: []domainexpense.Payer{{UserID: payer, Amount: value}},
		Splits: []domainexpense.Split{{UserID: debtor, Amount: value}}}
}

func TestOutstandingDebtIsBilateralNotOptimizerSuggested(t *testing.T) {
	a, b, c, d := uuid.UUID{15: 1}, uuid.UUID{15: 2}, uuid.UUID{15: 3}, uuid.UUID{15: 4}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben"), participant(c, "Cara"), participant(d, "Dee")}
	// A owes 50, D owes 50, B is owed 50, C is owed 50 — the optimizer pairs A→B and D→C, so the
	// legitimate A→C payment has no suggested edge to look up.
	expenses := expenseRows{paidFor(b, a, 50, "PHP"), paidFor(c, d, 50, "PHP")}
	trip := domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenses, Settlements: settlementRows{}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}, Trips: tripRow{trip: trip}})

	debts, err := service.Calculate(context.Background(), tripctx.TripContext{Trip: trip})
	if err != nil {
		t.Fatal(err)
	}
	for _, transfer := range debts.Debts {
		if transfer.FromUserID == a && transfer.ToUserID == c {
			t.Skip("optimizer happened to suggest A→C; this fixture no longer exercises the gap")
		}
	}

	outstanding, err := service.OutstandingDebt(context.Background(), trip.ID, a, c)
	if err != nil {
		t.Fatal(err)
	}
	if !outstanding.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("OutstandingDebt(A, C) = %s, want 50 — a legitimate unsuggested pairing must not be rejected", outstanding)
	}
}

func TestOutstandingDebtCapsAndDirection(t *testing.T) {
	a, b, c, d := uuid.UUID{15: 1}, uuid.UUID{15: 2}, uuid.UUID{15: 3}, uuid.UUID{15: 4}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben"), participant(c, "Cara"), participant(d, "Dee")}
	// A owes 100 and D owes 20; B is owed 30 and C is owed 90.
	expenses := expenseRows{paidFor(b, a, 30, "PHP"), paidFor(c, a, 70, "PHP"), paidFor(c, d, 20, "PHP")}
	trip := domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenses, Settlements: settlementRows{}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}, Trips: tripRow{trip: trip}})

	tests := []struct {
		name, want string
		from, to   uuid.UUID
	}{
		{name: "capped by what the payee is still owed", from: a, to: b, want: "30"},
		{name: "capped by what the payer still owes", from: a, to: c, want: "90"},
		{name: "creditor paying is settling nothing", from: b, to: c, want: "0"},
		{name: "paying a debtor is settling nothing", from: a, to: d, want: "0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outstanding, err := service.OutstandingDebt(context.Background(), trip.ID, test.from, test.to)
			if err != nil {
				t.Fatal(err)
			}
			if !outstanding.Equal(decimal.RequireFromString(test.want)) {
				t.Fatalf("OutstandingDebt() = %s, want %s", outstanding, test.want)
			}
		})
	}
}

// Test-plan item 7: the guard compares in base currency, so a foreign-currency settlement must be
// converted before it is measured against the outstanding debt.
func TestOutstandingDebtIsExpressedInBaseCurrency(t *testing.T) {
	a, b := uuid.UUID{15: 1}, uuid.UUID{15: 2}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben")}
	// One USD 2 expense at 50 PHP/USD — A owes B 100 PHP, not 2.
	expenses := expenseRows{paidFor(b, a, 2, "USD")}
	rates := []domainfx.Rate{{FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.NewFromInt(50)}}
	trip := domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenses, Settlements: settlementRows{}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(rates)}, Trips: tripRow{trip: trip}})

	outstanding, err := service.OutstandingDebt(context.Background(), trip.ID, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !outstanding.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("OutstandingDebt() = %s, want 100 (PHP, converted from USD 2)", outstanding)
	}
}

func TestFinalSettlementExposesFullBankDetailsOnlyToPayerOrPlanner(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben"), participant(c, "Cara")}
	participants[1].BankInfo = &domainparticipant.BankInfo{BankName: "Bank", AccountNumber: "12345678", AccountHolder: "Ben"}
	expenses := expenseRows{{Amount: decimal.NewFromInt(100), Currency: "PHP", Status: domainexpense.StatusApproved, Payers: []domainexpense.Payer{{UserID: b, Amount: decimal.NewFromInt(100)}}, Splits: []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(100)}}}}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenses, Settlements: settlementRows{}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})
	trip := domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}

	payerPlan, err := service.FinalSettlement(context.Background(), tripctx.TripContext{Trip: trip, Participant: domainparticipant.Participant{UserID: a, Role: domainparticipant.RoleParticipant}})
	if err != nil || len(payerPlan.Transfers) != 1 || payerPlan.Transfers[0].BankAccountNumber == nil || *payerPlan.Transfers[0].BankAccountNumber != "12345678" {
		t.Fatalf("payer plan=%+v err=%v", payerPlan, err)
	}
	bystanderPlan, err := service.FinalSettlement(context.Background(), tripctx.TripContext{Trip: trip, Participant: domainparticipant.Participant{UserID: c, Role: domainparticipant.RoleParticipant}})
	if err != nil || bystanderPlan.Transfers[0].BankAccountNumber != nil {
		t.Fatalf("bystander plan=%+v err=%v", bystanderPlan, err)
	}
	plannerPlan, err := service.FinalSettlement(context.Background(), tripctx.TripContext{Trip: trip, Participant: domainparticipant.Participant{UserID: c, Role: domainparticipant.RolePlanner}})
	if err != nil || plannerPlan.Transfers[0].BankAccountNumber == nil {
		t.Fatalf("planner plan=%+v err=%v", plannerPlan, err)
	}
}

func TestLedgerUnfilteredExcludesSelfTransactionsAndTracksRunningBalance(t *testing.T) {
	a, b, c := uuid.UUID{15: 1}, uuid.UUID{15: 2}, uuid.UUID{15: 3}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben"), participant(c, "Cara")}
	day1, day2 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	// A pays for something entirely for themselves - paid equals share, nets to zero, must not appear.
	selfOnly := domainexpense.Expense{ID: uuid.New(), ExpenseDate: day1, Description: "Personal snack", Currency: "PHP", Status: domainexpense.StatusApproved,
		Payers: []domainexpense.Payer{{UserID: a, Amount: decimal.NewFromInt(20)}}, Splits: []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(20)}}}
	// A pays 120 split equally three ways (40 each) - A is owed 80 overall.
	shared := domainexpense.Expense{ID: uuid.New(), ExpenseDate: day1, Description: "Group dinner", Currency: "PHP", Status: domainexpense.StatusApproved,
		Payers: []domainexpense.Payer{{UserID: a, Amount: decimal.NewFromInt(120)}},
		Splits: []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(40)}, {UserID: b, Amount: decimal.NewFromInt(40)}, {UserID: c, Amount: decimal.NewFromInt(40)}}}
	settlement := domainsettlement.Settlement{ID: uuid.New(), FromUserID: c, ToUserID: a, Amount: decimal.NewFromInt(10), Currency: "PHP", Status: domainsettlement.StatusApproved, CreatedAt: day2}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{selfOnly, shared}, Settlements: settlementRows{settlement}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})

	result, err := service.Ledger(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}}, balancedomain.LedgerFilter{MemberUserID: a})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %+v, want 2 (self-only expense excluded)", result.Entries)
	}
	if !result.Entries[0].Delta.Equal(decimal.NewFromInt(80)) || !result.Entries[0].RunningBalance.Equal(decimal.NewFromInt(80)) {
		t.Fatalf("entry[0] = %+v, want delta=80 running=80", result.Entries[0])
	}
	if result.Entries[0].Paid == nil || !result.Entries[0].Paid.Equal(decimal.NewFromInt(120)) || result.Entries[0].Share == nil || !result.Entries[0].Share.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("entry[0] paid/share = %+v/%+v, want 120/40", result.Entries[0].Paid, result.Entries[0].Share)
	}
	if !result.Entries[1].Delta.Equal(decimal.NewFromInt(-10)) || !result.Entries[1].RunningBalance.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("entry[1] (settlement received) = %+v, want delta=-10 running=70", result.Entries[1])
	}
	if !result.NetBalance.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("NetBalance = %s, want 70", result.NetBalance)
	}
}

func TestLedgerPairwiseSkipsSelfSharesAndComputesNetBetweenTwoMembers(t *testing.T) {
	a, b, c := uuid.UUID{15: 1}, uuid.UUID{15: 2}, uuid.UUID{15: 3}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben"), participant(c, "Cara")}
	expense := domainexpense.Expense{ID: uuid.New(), ExpenseDate: time.Now(), Description: "Group dinner", Currency: "PHP", Status: domainexpense.StatusApproved,
		Payers: []domainexpense.Payer{{UserID: a, Amount: decimal.NewFromInt(120)}},
		Splits: []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(40)}, {UserID: b, Amount: decimal.NewFromInt(40)}, {UserID: c, Amount: decimal.NewFromInt(40)}}}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{expense}, Settlements: settlementRows{}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})
	trip := tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}}

	// B owes A for the portion of B's share that A paid for.
	bAgainstA, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: b, AgainstUserID: &a})
	if err != nil {
		t.Fatal(err)
	}
	if len(bAgainstA.Entries) != 1 || !bAgainstA.Entries[0].Delta.Equal(decimal.NewFromInt(-40)) {
		t.Fatalf("B against A = %+v, want a single -40 entry", bAgainstA.Entries)
	}
	if bAgainstA.Entries[0].CounterpartyUserID == nil || *bAgainstA.Entries[0].CounterpartyUserID != a {
		t.Fatalf("B against A counterparty = %+v, want A", bAgainstA.Entries[0].CounterpartyUserID)
	}

	// Symmetric from A's side.
	aAgainstB, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: a, AgainstUserID: &b})
	if err != nil {
		t.Fatal(err)
	}
	if len(aAgainstB.Entries) != 1 || !aAgainstB.Entries[0].Delta.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("A against B = %+v, want a single +40 entry", aAgainstB.Entries)
	}

	// B and C never transacted directly on this expense (neither paid the other's share).
	bAgainstC, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: b, AgainstUserID: &c})
	if err != nil {
		t.Fatal(err)
	}
	if len(bAgainstC.Entries) != 0 {
		t.Fatalf("B against C = %+v, want no entries", bAgainstC.Entries)
	}
}

func TestLedgerSettlementDirectionsAndFiltering(t *testing.T) {
	a, b, c := uuid.UUID{15: 1}, uuid.UUID{15: 2}, uuid.UUID{15: 3}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben"), participant(c, "Cara")}
	settlement := domainsettlement.Settlement{ID: uuid.New(), FromUserID: a, ToUserID: b, Amount: decimal.NewFromInt(50), Currency: "PHP", Status: domainsettlement.StatusApproved, CreatedAt: time.Now()}
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{}, Settlements: settlementRows{settlement}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})
	trip := tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "PHP"}}

	sender, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: a})
	if err != nil || len(sender.Entries) != 1 || !sender.Entries[0].Delta.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("sender ledger = %+v, err=%v, want a single +50 entry", sender.Entries, err)
	}
	receiver, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: b})
	if err != nil || len(receiver.Entries) != 1 || !receiver.Entries[0].Delta.Equal(decimal.NewFromInt(-50)) {
		t.Fatalf("receiver ledger = %+v, err=%v, want a single -50 entry", receiver.Entries, err)
	}
	filtered, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: a, AgainstUserID: &b})
	if err != nil || len(filtered.Entries) != 1 {
		t.Fatalf("A against B = %+v, err=%v, want the settlement included", filtered.Entries, err)
	}
	unrelated, err := service.Ledger(context.Background(), trip, balancedomain.LedgerFilter{MemberUserID: a, AgainstUserID: &c})
	if err != nil || len(unrelated.Entries) != 0 {
		t.Fatalf("A against C = %+v, err=%v, want no entries (settlement was with B)", unrelated.Entries, err)
	}
}

func TestLedgerHonorsChargedAmountOverride(t *testing.T) {
	a, b := uuid.UUID{15: 1}, uuid.UUID{15: 2}
	participants := participantRows{participant(a, "Ana"), participant(b, "Ben")}
	// 1500 THB paid by A, actually charged 840000 IDR - the per-expense override rate, not any
	// trip-configured rate (there is none here).
	expense := chargedExpense("THB", 1500, a, 840000, "IDR", []domainexpense.Split{{UserID: a, Amount: decimal.NewFromInt(750)}, {UserID: b, Amount: decimal.NewFromInt(750)}})
	service := balancedomain.NewService(balancedomain.Dependencies{Expenses: expenseRows{expense}, Settlements: settlementRows{}, Participants: participants, FX: tableProvider{table: fxdomain.NewRateTable(nil)}})

	result, err := service.Ledger(context.Background(), tripctx.TripContext{Trip: domaintrip.Trip{ID: uuid.New(), BaseCurrency: "IDR"}}, balancedomain.LedgerFilter{MemberUserID: a})
	if err != nil {
		t.Fatalf("Ledger() error = %v, want no error even without a configured THB→IDR rate", err)
	}
	if len(result.Entries) != 1 || !result.Entries[0].Delta.Equal(decimal.NewFromInt(420000)) {
		t.Fatalf("entries = %+v, want a single +420000 entry (840000 charged - 420000 own share)", result.Entries)
	}
}
