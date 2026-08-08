package balance_test

import (
	"context"
	"strings"
	"testing"

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
