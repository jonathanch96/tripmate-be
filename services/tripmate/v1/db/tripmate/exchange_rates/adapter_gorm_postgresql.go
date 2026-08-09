package exchange_rates

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func New(db *gorm.DB) *adapterGormPostgresql { return &adapterGormPostgresql{db: db} }

func (a *adapterGormPostgresql) ListEffective(ctx context.Context, tripID uuid.UUID) ([]domainfx.Rate, error) {
	var models []ExchangeRate
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).Where("trip_id = ? OR trip_id IS NULL", tripID).
		Order("from_currency, to_currency, trip_id NULLS FIRST").Find(&models).Error
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	byPair := make(map[string]domainfx.Rate, len(models))
	for _, model := range models {
		row := toDomain(model)
		key := row.FromCurrency + "\x00" + row.ToCurrency
		current, exists := byPair[key]
		if !exists || current.TripID == nil || row.TripID != nil {
			byPair[key] = row
		}
	}
	result := make([]domainfx.Rate, 0, len(byPair))
	for _, model := range models {
		key := model.FromCurrency + "\x00" + model.ToCurrency
		row, exists := byPair[key]
		if exists && row.ID == model.ID {
			result = append(result, row)
			delete(byPair, key)
		}
	}
	return result, nil
}

func (a *adapterGormPostgresql) Get(ctx context.Context, tripID *uuid.UUID, from, to string) (*domainfx.Rate, error) {
	var model ExchangeRate
	query := appdb.FromContext(ctx, a.db).WithContext(ctx).Where("from_currency = ? AND to_currency = ?", normalize(from), normalize(to))
	if tripID == nil {
		query = query.Where("trip_id IS NULL")
	} else {
		query = query.Where("trip_id = ?", *tripID)
	}
	if err := query.First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.Newf("EXCHANGE_RATE_MISSING", "Exchange rate %s→%s is required", normalize(from), normalize(to))
		}
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	row := toDomain(model)
	return &row, nil
}

func (a *adapterGormPostgresql) Upsert(ctx context.Context, row domainfx.Rate) (*domainfx.Rate, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.FromCurrency, row.ToCurrency = normalize(row.FromCurrency), normalize(row.ToCurrency)
	if row.EffectiveAt.IsZero() {
		row.EffectiveAt = time.Now().UTC()
	}
	model := fromDomain(row)
	conflict := "(trip_id, from_currency, to_currency) WHERE trip_id IS NOT NULL"
	if row.TripID == nil {
		conflict = "(from_currency, to_currency) WHERE trip_id IS NULL"
	}
	query := `INSERT INTO tripmate.exchange_rates
		(id, trip_id, from_currency, to_currency, rate, is_final, source, effective_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT ` + conflict + ` DO UPDATE SET rate = EXCLUDED.rate, is_final = EXCLUDED.is_final,
			source = EXCLUDED.source, effective_at = EXCLUDED.effective_at, updated_at = now()
		RETURNING *`
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Raw(query, model.ID, model.TripID, model.FromCurrency, model.ToCurrency, model.Rate, model.IsFinal, model.Source, model.EffectiveAt).Scan(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) DeleteTripPair(ctx context.Context, tripID uuid.UUID, from, to string) error {
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).
		Where("trip_id = ? AND from_currency = ? AND to_currency = ?", tripID, normalize(from), normalize(to)).
		Delete(&ExchangeRate{}).Error
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) ListGlobal(ctx context.Context) ([]domainfx.Rate, error) {
	var models []ExchangeRate
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Where("trip_id IS NULL").Order("from_currency, to_currency").Find(&models).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := make([]domainfx.Rate, len(models))
	for i := range models {
		result[i] = toDomain(models[i])
	}
	return result, nil
}

func normalize(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
