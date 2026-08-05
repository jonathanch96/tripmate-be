package money

import (
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

func TestSplitEqualExamples(t *testing.T) {
	t.Parallel()
	tests := []struct {
		total, currency string
		n               int
		want            []string
	}{
		{"100.00", "USD", 3, []string{"33.34", "33.33", "33.33"}},
		{"0.01", "USD", 3, []string{"0.01", "0", "0"}},
		{"1000", "IDR", 3, []string{"334", "333", "333"}},
	}
	for _, test := range tests {
		got := SplitEqual(decimal.RequireFromString(test.total), test.n, test.currency)
		for i := range got {
			if !got[i].Equal(decimal.RequireFromString(test.want[i])) {
				t.Fatalf("SplitEqual(%s, %d, %s)[%d] = %s, want %s", test.total, test.n, test.currency, i, got[i], test.want[i])
			}
		}
	}
}

func TestSplitEqualProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		minor := rapid.Int64Range(0, 100_000_000_000).Draw(t, "minor")
		n := rapid.IntRange(1, 50).Draw(t, "n")
		currency := rapid.SampledFrom([]string{"USD", "PHP", "EUR", "IDR", "JPY"}).Draw(t, "currency")
		scale := DisplayScale(currency)
		total := decimal.New(minor, -2)
		parts := SplitEqual(total, n, currency)
		if !Sum(parts...).Equal(Round(total, scale)) {
			t.Fatalf("parts sum %s, want %s", Sum(parts...), Round(total, scale))
		}
		unit := decimal.New(1, -scale)
		for i := range parts {
			for j := range parts {
				if parts[i].Sub(parts[j]).Abs().GreaterThan(unit) {
					t.Fatalf("parts differ by more than one minor unit: %v", parts)
				}
			}
			if scale == 0 && parts[i].Exponent() < 0 && !parts[i].Equal(parts[i].Round(0)) {
				t.Fatalf("zero-scale currency has fractional part: %s", parts[i])
			}
		}
		equalWeights := make([]decimal.Decimal, n)
		for i := range equalWeights {
			equalWeights[i] = decimal.NewFromInt(1)
		}
		proportional := SplitProportional(total, equalWeights, currency)
		for i := range parts {
			if !parts[i].Equal(proportional[i]) {
				t.Fatalf("equal and proportional splits differ: %v != %v", parts, proportional)
			}
		}
	})
}

func TestRoundUsesHalfEven(t *testing.T) {
	t.Parallel()
	if got := Round(decimal.RequireFromString("2.345"), 2); !got.Equal(decimal.RequireFromString("2.34")) {
		t.Fatalf("got %s", got)
	}
	if got := Round(decimal.RequireFromString("2.355"), 2); !got.Equal(decimal.RequireFromString("2.36")) {
		t.Fatalf("got %s", got)
	}
}
