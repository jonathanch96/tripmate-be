package expense

import (
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
		{name: "item requires at least one line", input: SplitInput{Amount: amount("100"), Currency: "PHP", SplitType: domainexpense.SplitItem}, errorCode: "VALIDATION_FAILED"},
		{name: "item charges each line to whoever shared it", input: SplitInput{Currency: "PHP", SplitType: domainexpense.SplitItem, Items: []ItemAssignment{
			{Amount: amount("300"), UserIDs: []uuid.UUID{participants[0]}},
			{Amount: amount("200"), UserIDs: []uuid.UUID{participants[1]}},
		}}, want: []string{"300", "200"}},
		{name: "item shared by three allocates to the cent", input: SplitInput{Currency: "PHP", SplitType: domainexpense.SplitItem, Items: []ItemAssignment{
			{Amount: amount("100"), UserIDs: []uuid.UUID{participants[0], participants[1], participants[2]}},
		}}, want: []string{"33.34", "33.33", "33.33"}},
		{name: "item spreads extras in proportion to what was eaten", input: SplitInput{Currency: "PHP", SplitType: domainexpense.SplitItem, Extras: amount("100"), Items: []ItemAssignment{
			{Amount: amount("300"), UserIDs: []uuid.UUID{participants[0]}},
			{Amount: amount("100"), UserIDs: []uuid.UUID{participants[1]}},
		}}, want: []string{"375", "125"}},
		{name: "item leaves a diner who ate nothing free of extras", input: SplitInput{Currency: "PHP", SplitType: domainexpense.SplitItem, Extras: amount("50"), Items: []ItemAssignment{
			{Amount: amount("200"), UserIDs: []uuid.UUID{participants[0]}},
			{Amount: decimal.Zero, UserIDs: []uuid.UUID{participants[1]}},
		}}, want: []string{"250", "0"}},
		{name: "item rejects an unassigned line", input: SplitInput{Currency: "PHP", SplitType: domainexpense.SplitItem, Items: []ItemAssignment{
			{Amount: amount("100")},
		}}, errorCode: "VALIDATION_FAILED"},
		{name: "item rejects a stated total that does not match the bill", input: SplitInput{Amount: amount("999"), Currency: "PHP", SplitType: domainexpense.SplitItem, Items: []ItemAssignment{
			{Amount: amount("100"), UserIDs: []uuid.UUID{participants[0]}},
		}}, errorCode: "SPLIT_SUM_MISMATCH"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CalculateSplits(test.input)
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

func TestEqualSplitRejectsDuplicateParticipant(t *testing.T) {
	id := uuid.New()
	_, err := CalculateSplits(SplitInput{Amount: decimal.NewFromInt(10), Currency: "PHP", SplitType: domainexpense.SplitEqual, Participants: []uuid.UUID{id, id}})
	if !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("CalculateSplits(duplicate participant) error = %v", err)
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

// Test-plan item 4: whatever the bill looks like — however many lines, however they are shared,
// whatever the tax — the splits must add up to items + extras exactly, in every currency scale.
func TestItemSplitAlwaysSumsToTheBill(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		currency := rapid.SampledFrom([]string{"PHP", "USD", "IDR", "JPY"}).Draw(t, "currency")
		scale := displayScale(currency)
		diners := rapid.IntRange(1, 8).Draw(t, "diners")
		ids := make([]uuid.UUID, diners)
		for index := range ids {
			ids[index] = uuid.MustParse("00000000-0000-0000-0000-" + leftPad(index+1))
		}
		lines := rapid.IntRange(1, 12).Draw(t, "lines")
		items := make([]ItemAssignment, lines)
		itemsTotal := decimal.Zero
		for index := range items {
			price := decimal.New(rapid.Int64Range(0, 5_000_000).Draw(t, "price"), -scale)
			shared := rapid.IntRange(1, diners).Draw(t, "shared")
			// Pick `shared` distinct diners starting from a random offset.
			offset := rapid.IntRange(0, diners-1).Draw(t, "offset")
			assignees := make([]uuid.UUID, shared)
			for i := range assignees {
				assignees[i] = ids[(offset+i)%diners]
			}
			items[index] = ItemAssignment{Amount: price, UserIDs: assignees}
			itemsTotal = itemsTotal.Add(price)
		}
		extras := decimal.New(rapid.Int64Range(0, 2_000_000).Draw(t, "extras"), -scale)

		splits, err := CalculateSplits(SplitInput{Currency: currency, SplitType: domainexpense.SplitItem, Items: items, Extras: extras})
		if err != nil {
			t.Fatal(err)
		}
		parts := make([]decimal.Decimal, len(splits))
		for index := range splits {
			parts[index] = splits[index].Amount
			if parts[index].IsNegative() {
				t.Fatalf("split[%d] = %s is negative", index, parts[index])
			}
		}
		want := itemsTotal.Add(extras).RoundBank(scale)
		if !sum(parts).Equal(want) {
			t.Fatalf("split sum = %s, want %s", sum(parts), want)
		}
		// Nobody may appear twice, and the order must be deterministic by user id.
		seen := make(map[uuid.UUID]struct{}, len(splits))
		for index, split := range splits {
			if _, exists := seen[split.UserID]; exists {
				t.Fatalf("user %s appears twice", split.UserID)
			}
			seen[split.UserID] = struct{}{}
			if index > 0 && splits[index-1].UserID.String() >= split.UserID.String() {
				t.Fatalf("splits are not sorted by user id: %s then %s", splits[index-1].UserID, split.UserID)
			}
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
