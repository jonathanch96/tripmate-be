package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"
)

// rateScale is how many decimals an exchange rate is shown with (1 IDR = 0.00946 JPY needs them).
const rateScale = 8

// conversion is the currency an amount is recorded in and, when that is not the trip's base
// currency, the rate balances will use to convert it.
type conversion struct {
	Currency, Base string
	// Rate is how many base-currency units one unit of Currency is worth; zero for the base currency.
	Rate decimal.Decimal
}

// currencyFor settles the currency an expense or repayment is recorded in. Amounts stay in the
// currency the user gave; a currency other than the base needs an exchange rate on the trip first,
// because balances convert everything into the base currency.
func (c *controller) currencyFor(ctx context.Context, k *caller, tc *tripctx.TripContext, raw string) (conversion, error) {
	base := strings.ToUpper(tc.Trip.BaseCurrency)
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		code = base
	}
	if !money.IsSupportedCurrency(code) {
		return conversion{}, fmt.Errorf("nothing was saved: %q is not a currency TripMate supports. Use the 3-letter ISO 4217 code of the currency on the bill (e.g. IDR, JPY, USD); ask the user if unsure", raw)
	}
	if code == base {
		return conversion{Currency: code, Base: base}, nil
	}
	if !tc.Trip.AllowsCurrency(code) {
		return conversion{}, fmt.Errorf("nothing was saved: trip %s only records amounts in its base currency %s, because multiple currencies are turned off in its settings. "+
			"Tell the user and ask whether to enter the amount converted to %s (and which rate they used), or have the trip planner turn on multiple currencies in the TripMate trip settings first",
			tc.Trip.Name, base, base)
	}
	table, err := c.deps.FX.EffectiveTable(ctx, tc.Trip.ID)
	if err != nil {
		return conversion{}, toolError(err)
	}
	rate, err := table.Convert(decimal.NewFromInt(1), code, base)
	if err != nil {
		return conversion{}, c.missingRate(ctx, k, tc, code, base)
	}
	return conversion{Currency: code, Base: base, Rate: rate}, nil
}

func (c *controller) missingRate(ctx context.Context, k *caller, tc *tripctx.TripContext, code, base string) error {
	head := fmt.Sprintf("nothing was saved: trip %s has no exchange rate from %s to its base currency %s yet, and balances need one to convert this amount. ", tc.Trip.Name, code, base)
	if tc.Participant.Role == domainparticipant.RolePlanner {
		return errors.New(head + fmt.Sprintf("Ask the user how much 1 %s is worth in %s (you may suggest today's market rate, but use the number they confirm), "+
			"save it with set_exchange_rate (currency %s, rate_to_base), then call this tool again with the same details. Keep the amounts in %s - do not convert them yourself",
			code, base, code, code))
	}
	planners := make([]string, 0, 1)
	if parts, err := c.deps.Participants.List(ctx, k.who.UserID, tc.Trip.Code); err == nil {
		for _, part := range parts {
			if part.Role == domainparticipant.RolePlanner {
				planners = append(planners, part.EffectiveName())
			}
		}
	}
	who := "the trip planner"
	if len(planners) > 0 {
		who += " (" + strings.Join(planners, ", ") + ")"
	}
	return errors.New(head + fmt.Sprintf("Only %s can set exchange rates. Tell the user, and ask them either to have the planner add the %s rate in TripMate and then try again, "+
		"or to give the amount in %s instead", who, code, base))
}

// out describes the conversion for tool output, or nil for an amount in the base currency.
func (v conversion) out(amount decimal.Decimal) *conversionOut {
	if v.Rate.IsZero() {
		return nil
	}
	return &conversionOut{BaseCurrency: v.Base, Rate: rateText(v.Currency, v.Rate, v.Base),
		Amount: amount.Mul(v.Rate).StringFixedBank(money.DisplayScale(v.Base))}
}

func rateText(from string, rate decimal.Decimal, to string) string {
	return fmt.Sprintf("1 %s = %s %s", from, rate.RoundBank(rateScale).String(), to)
}

// exchangeRates lists the trip's rates as "how much one unit is worth in the base currency".
func exchangeRates(table *fxdomain.RateTable, currencies []string, base string) []rateOut {
	result := make([]rateOut, 0, len(currencies))
	for _, code := range currencies {
		if rate, err := table.Convert(decimal.NewFromInt(1), code, base); err == nil {
			result = append(result, rateOut{Currency: code, RateToBase: rate.RoundBank(rateScale).String()})
		}
	}
	return result
}

func (c *controller) setExchangeRate(ctx context.Context, req *mcp.CallToolRequest, in setRateInput) (*mcp.CallToolResult, savedRateOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, savedRateOut{}, err
	}
	if err := k.requireWrite(); err != nil {
		return nil, savedRateOut{}, err
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, savedRateOut{}, toolError(err)
	}
	base := strings.ToUpper(tc.Trip.BaseCurrency)
	code := strings.ToUpper(strings.TrimSpace(in.Currency))
	switch {
	case !money.IsSupportedCurrency(code):
		return nil, savedRateOut{}, fmt.Errorf("currency must be a 3-letter ISO 4217 code TripMate supports (e.g. JPY), got %q", in.Currency)
	case code == base:
		return nil, savedRateOut{}, fmt.Errorf("%s is already the trip's base currency, so it needs no exchange rate", base)
	case !tc.Trip.AllowsCurrency(code):
		return nil, savedRateOut{}, fmt.Errorf("trip %s only uses its base currency %s; the planner has to turn on multiple currencies in the TripMate trip settings first", tc.Trip.Name, base)
	}
	rate, err := in.RateToBase.Decimal("rate_to_base")
	if err != nil {
		return nil, savedRateOut{}, err
	}
	if !rate.IsPositive() {
		return nil, savedRateOut{}, fmt.Errorf("rate_to_base must be more than zero: how many %s one %s is worth", base, code)
	}
	var previous string
	if table, err := c.deps.FX.EffectiveTable(ctx, tc.Trip.ID); err == nil {
		if old, err := table.Convert(decimal.NewFromInt(1), code, base); err == nil {
			previous = rateText(code, old, base)
		}
	}
	if _, err := c.deps.FX.SetTripRate(ctx, k.who, *tc, fxdomain.SetRateInput{FromCurrency: code, ToCurrency: base, Rate: rate}); err != nil {
		return nil, savedRateOut{}, toolError(err)
	}
	text := rateText(code, rate, base)
	message := "Saved " + text + " for this trip. Now retry the expense or repayment that needed it, with its amounts still in " + code + "."
	if previous != "" {
		message = "Saved " + text + " for this trip, replacing " + previous + ". Balances now use it for every " + code + " amount on the trip."
	}
	return nil, savedRateOut{Currency: code, BaseCurrency: base, RateToBase: rate.RoundBank(rateScale).String(), Rate: text, Message: message}, nil
}

// rateCurrencies lists, once each and in order, the currencies the stored rates pair with base.
func rateCurrencies(rates []domainfx.Rate, base string) []string {
	base = strings.ToUpper(base)
	seen := map[string]bool{}
	result := make([]string, 0, len(rates))
	for _, rate := range rates {
		from, to := strings.ToUpper(rate.FromCurrency), strings.ToUpper(rate.ToCurrency)
		other := ""
		switch base {
		case to:
			other = from
		case from:
			other = to
		}
		if other != "" && other != base && !seen[other] {
			seen[other] = true
			result = append(result, other)
		}
	}
	return result
}
