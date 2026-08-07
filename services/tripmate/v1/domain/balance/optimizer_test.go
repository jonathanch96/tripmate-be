package balance_test

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

func TestOptimizeUsesDeterministicUUIDTieBreak(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	c := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	d := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	balances := []domainbalance.ParticipantBalance{{UserID: d, NetBalance: decimal.NewFromInt(5)}, {UserID: b, NetBalance: decimal.NewFromInt(-5)}, {UserID: c, NetBalance: decimal.NewFromInt(5)}, {UserID: a, NetBalance: decimal.NewFromInt(-5)}}
	want := []domainbalance.Transfer{{FromUserID: a, ToUserID: c, Amount: decimal.NewFromInt(5)}, {FromUserID: b, ToUserID: d, Amount: decimal.NewFromInt(5), Currency: "PHP"}}
	want[0].Currency = "PHP"
	if got := balancedomain.Optimize(balances, "PHP"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Optimize() = %+v, want %+v", got, want)
	}
}

func TestOptimizeProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 12).Draw(t, "participants")
		amounts := make([]int64, n)
		var total int64
		for i := 0; i < n-1; i++ {
			amounts[i] = rapid.Int64Range(-1_000_000, 1_000_000).Draw(t, "amount")
			total += amounts[i]
		}
		amounts[n-1] = -total
		balances := make([]domainbalance.ParticipantBalance, n)
		for i := range balances {
			balances[i] = domainbalance.ParticipantBalance{UserID: uuid.UUID{15: byte(i + 1)}, NetBalance: decimal.NewFromInt(amounts[i]).Div(decimal.NewFromInt(100))}
		}
		transfers := balancedomain.Optimize(balances, "PHP")
		if len(transfers) > n-1 {
			t.Fatalf("transfer count %d > %d", len(transfers), n-1)
		}
		remaining := make(map[uuid.UUID]decimal.Decimal, n)
		for _, balance := range balances {
			remaining[balance.UserID] = balance.NetBalance
		}
		for _, transfer := range transfers {
			if !transfer.Amount.IsPositive() || transfer.FromUserID == transfer.ToUserID {
				t.Fatalf("invalid transfer: %+v", transfer)
			}
			remaining[transfer.FromUserID] = remaining[transfer.FromUserID].Add(transfer.Amount)
			remaining[transfer.ToUserID] = remaining[transfer.ToUserID].Sub(transfer.Amount)
		}
		for userID, value := range remaining {
			if !money.IsZero(value) {
				t.Fatalf("user %s remains at %s", userID, value)
			}
		}

		shuffled := append([]domainbalance.ParticipantBalance(nil), balances...)
		rand.New(rand.NewSource(42)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		again := balancedomain.Optimize(shuffled, "PHP")
		if !reflect.DeepEqual(transfers, again) {
			t.Fatalf("non-deterministic output: %+v != %+v", transfers, again)
		}
	})
}

func TestOptimizeOutputIsStableAcrossOneHundredShuffles(t *testing.T) {
	balances := []domainbalance.ParticipantBalance{
		{UserID: uuid.UUID{15: 1}, NetBalance: decimal.NewFromInt(-9)},
		{UserID: uuid.UUID{15: 2}, NetBalance: decimal.NewFromInt(-4)},
		{UserID: uuid.UUID{15: 3}, NetBalance: decimal.NewFromInt(6)},
		{UserID: uuid.UUID{15: 4}, NetBalance: decimal.NewFromInt(7)},
	}
	want := balancedomain.Optimize(balances, "PHP")
	for seed := int64(0); seed < 100; seed++ {
		shuffled := append([]domainbalance.ParticipantBalance(nil), balances...)
		rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := balancedomain.Optimize(shuffled, "PHP")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffle %d changed output: %+v", seed, got)
		}
	}
}
