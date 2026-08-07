package fx

import (
	"strings"

	"github.com/jblabs/tripmate-be/pkg/apperror"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	"github.com/shopspring/decimal"
)

type pairKey struct{ from, to string }

type RateTable struct {
	rates map[pairKey]domainfx.Rate
}

func NewRateTable(rates []domainfx.Rate) *RateTable {
	table := &RateTable{rates: make(map[pairKey]domainfx.Rate, len(rates))}
	for _, rate := range rates {
		rate.FromCurrency = normalize(rate.FromCurrency)
		rate.ToCurrency = normalize(rate.ToCurrency)
		key := pairKey{from: rate.FromCurrency, to: rate.ToCurrency}
		current, exists := table.rates[key]
		if exists && current.TripID != nil && rate.TripID == nil {
			continue
		}
		table.rates[key] = rate
	}
	return table
}

func (t *RateTable) Convert(amount decimal.Decimal, from, to string) (decimal.Decimal, error) {
	from, to = normalize(from), normalize(to)
	if from == to {
		return amount, nil
	}
	if rate, ok := t.rates[pairKey{from: from, to: to}]; ok {
		return amount.Mul(rate.Rate), nil
	}
	if rate, ok := t.rates[pairKey{from: to, to: from}]; ok {
		return amount.DivRound(rate.Rate, 12), nil
	}
	return decimal.Zero, apperror.Newf("EXCHANGE_RATE_MISSING", "Exchange rate %s→%s is required", from, to)
}

func normalize(currency string) string { return strings.ToUpper(strings.TrimSpace(currency)) }
