package expense

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

func amount(value string) decimal.Decimal { return decimal.RequireFromString(value) }

func participant(number byte) uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-00000000000" + string(rune('0'+number)))
}

func TestCalculateSplitsRequiredTable(t *testing.T) {
	participants := []uuid.UUID{participant(1), participant(2), participant(3), participant(4)}
	tests := []struct {
		name      string
		input     SplitInput
		want      []string
		errorCode string
		notBuilt  bool
	}{
		{name: "equal clean", input: SplitInput{Amount: amount("100.00"), Currency: "PHP", SplitType: domainexpense.SplitEqual, Participants: participants}, want: []string{"25", "25", "25", "25"}},
		{name: "equal remainder follows uuid order", input: SplitInput{Amount: amount("100.00"), Currency: "PHP", SplitType: domainexpense.SplitEqual, Participants: participants[:3]}, want: []string{"33.34", "33.33", "33.33"}},
		{name: "equal sub-cent", input: SplitInput{Amount: amount("0.01"), Currency: "PHP", SplitType: domainexpense.SplitEqual, Participants: participants[:3]}, want: []string{"0.01", "0", "0"}},
		{name: "equal zero-decimal currency", input: SplitInput{Amount: amount("1000"), Currency: "IDR", SplitType: domainexpense.SplitEqual, Participants: participants[:3]}, want: []string{"334", "333", "333"}},
		{name: "equal demo expense", input: SplitInput{Amount: amount("8500"), Currency: "PHP", SplitType: domainexpense.SplitEqual, Participants: participants}, want: []string{"2125", "2125", "2125", "2125"}},
		{name: "manual valid", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitManual, Manual: map[uuid.UUID]decimal.Decimal{participants[0]: amount("60"), participants[1]: amount("40")}}, want: []string{"60", "40"}},
		{name: "manual mismatch", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitManual, Manual: map[uuid.UUID]decimal.Decimal{participants[0]: amount("60"), participants[1]: amount("39.99")}}, errorCode: "SPLIT_SUM_MISMATCH"},
		{name: "manual zero allowed", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitManual, Manual: map[uuid.UUID]decimal.Decimal{participants[0]: amount("100"), participants[1]: decimal.Zero}}, want: []string{"100", "0"}},
		{name: "manual negative rejected", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitManual, Manual: map[uuid.UUID]decimal.Decimal{participants[0]: amount("-10"), participants[1]: amount("110")}}, errorCode: "VALIDATION_FAILED"},
		{name: "equal requires participants", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitEqual}, errorCode: "VALIDATION_FAILED"},
		{name: "item deferred", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitItem}, notBuilt: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CalculateSplits(test.input)
			if test.notBuilt {
				if !errors.Is(err, ErrNotImplemented) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if test.errorCode != "" {
				if !apperror.Is(err, test.errorCode) {
					t.Fatalf("error = %v, want %s", err, test.errorCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("splits = %+v", got)
			}
			for index := range got {
				if !got[index].Amount.Equal(amount(test.want[index])) {
					t.Fatalf("split[%d] = %s, want %s", index, got[index].Amount, test.want[index])
				}
			}
		})
	}
}

func TestValidatePayersRequiredTable(t *testing.T) {
	first, second, outsider := participant(1), participant(2), participant(4)
	valid := []domainexpense.Payer{{UserID: first, Amount: amount("8000")}, {UserID: second, Amount: amount("4000")}}
	if err := ValidatePayers(amount("12000"), "PHP", valid); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePayers(amount("12000"), "PHP", []domainexpense.Payer{{UserID: first, Amount: amount("8000")}, {UserID: second, Amount: amount("3999")}}); !apperror.Is(err, "PAYER_SUM_MISMATCH") {
		t.Fatalf("mismatch = %v", err)
	}
	if err := ValidateExpenseParticipants([]uuid.UUID{first, second}, valid, []domainexpense.Split{{UserID: outsider, Amount: amount("12000")}}); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("outsider = %v", err)
	}
	if err := ValidatePayers(amount("12000"), "PHP", []domainexpense.Payer{{UserID: first, Amount: amount("6000")}, {UserID: first, Amount: amount("6000")}}); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("duplicate = %v", err)
	}
}

func TestSplitAndPayerSumsAlwaysMatch(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currency := rapid.SampledFrom([]string{"PHP", "USD", "IDR", "JPY"}).Draw(t, "currency")
		minor := rapid.Int64Range(1, 100_000_000).Draw(t, "minor")
		count := rapid.IntRange(1, 9).Draw(t, "count")
		total := decimal.New(minor, -displayScale(currency))
		ids := make([]uuid.UUID, count)
		for index := range ids {
			ids[index] = uuid.MustParse("00000000-0000-0000-0000-" + leftPad(index+1))
		}
		splits, err := CalculateSplits(SplitInput{Amount: total, Currency: currency, SplitType: domainexpense.SplitEqual, Participants: ids})
		if err != nil {
			t.Fatal(err)
		}
		parts := make([]decimal.Decimal, len(splits))
		payers := []domainexpense.Payer{{UserID: ids[0], Amount: total}}
		for index := range splits {
			parts[index] = splits[index].Amount
		}
		if !sum(parts).Equal(total.RoundBank(displayScale(currency))) {
			t.Fatalf("split sum = %s, total = %s", sum(parts), total)
		}
		if err := ValidatePayers(total, currency, payers); err != nil {
			t.Fatal(err)
		}
	})
}

func leftPad(value int) string {
	text := decimal.NewFromInt(int64(value)).String()
	for len(text) < 12 {
		text = "0" + text
	}
	return text
}
