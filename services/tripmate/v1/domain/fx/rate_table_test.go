package fx_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	"github.com/shopspring/decimal"
)

func TestRateTableConvert(t *testing.T) {
	tripID := uuid.New()
	table := fxdomain.NewRateTable([]domainfx.Rate{
		{FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.RequireFromString("56.5")},
		{TripID: &tripID, FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.RequireFromString("57.25")},
		{FromCurrency: "EUR", ToCurrency: "PHP", Rate: decimal.RequireFromString("61")},
	})
	tests := []struct {
		name, amount, from, to, want string
	}{
		{"identity", "123.456789", "PHP", "php", "123.456789"},
		{"direct trip rate shadows global", "2", "usd", "PHP", "114.5"},
		{"reverse", "122", "PHP", "EUR", "2"},
		{"zero", "0", "USD", "PHP", "0"},
		{"huge", "999999999999.123456", "EUR", "PHP", "60999999999946.530816"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := table.Convert(decimal.RequireFromString(test.amount), test.from, test.to)
			if err != nil || !got.Equal(decimal.RequireFromString(test.want)) {
				t.Fatalf("Convert() = %s, %v; want %s", got, err, test.want)
			}
		})
	}
}

func TestRateTableMissingPairIsAnError(t *testing.T) {
	table := fxdomain.NewRateTable(nil)
	_, err := table.Convert(decimal.NewFromInt(10), "USD", "JPY")
	if !apperror.Is(err, "EXCHANGE_RATE_MISSING") || err.Error() == "" {
		t.Fatalf("Convert(missing) error = %v", err)
	}
}
