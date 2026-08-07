package exchange_rates

import domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"

func fromDomain(row domainfx.Rate) ExchangeRate {
	return ExchangeRate{ID: row.ID, TripID: row.TripID, FromCurrency: row.FromCurrency, ToCurrency: row.ToCurrency, Rate: row.Rate, IsFinal: row.IsFinal, Source: string(row.Source), EffectiveAt: row.EffectiveAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func toDomain(row ExchangeRate) domainfx.Rate {
	return domainfx.Rate{ID: row.ID, TripID: row.TripID, FromCurrency: row.FromCurrency, ToCurrency: row.ToCurrency, Rate: row.Rate, IsFinal: row.IsFinal, Source: domainfx.Source(row.Source), EffectiveAt: row.EffectiveAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
