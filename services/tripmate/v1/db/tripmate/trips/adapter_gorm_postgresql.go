package trips

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}

func (a *adapterGormPostgresql) SetFinalized(ctx context.Context, id uuid.UUID, finalized bool) error {
	values := map[string]any{"is_finalized": finalized, "version": gorm.Expr("version + 1"), "updated_at": gorm.Expr("now()")}
	if finalized {
		values["finalized_at"] = gorm.Expr("now()")
	} else {
		values["finalized_at"] = nil
	}
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Trip{}).Where("id = ?", id).Updates(values)
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("TRIP_NOT_FOUND")
	}
	return nil
}

func (a *adapterGormPostgresql) SetArchived(ctx context.Context, id uuid.UUID, archived bool) error {
	values := map[string]any{"is_archived": archived, "version": gorm.Expr("version + 1"), "updated_at": gorm.Expr("now()")}
	if archived {
		values["archived_at"] = gorm.Expr("now()")
	} else {
		values["archived_at"] = nil
	}
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Trip{}).Where("id = ?", id).Updates(values)
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("TRIP_NOT_FOUND")
	}
	return nil
}

func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) Create(ctx context.Context, entity *domaintrip.Trip) (*domaintrip.Trip, error) {
	model := fromDomain(*entity)
	if err := a.db.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetByID(ctx context.Context, id uuid.UUID) (*domaintrip.Trip, error) {
	var model Trip
	if err := a.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetByCode(ctx context.Context, code string) (*domaintrip.Trip, error) {
	var model Trip
	if err := a.db.WithContext(ctx).First(&model, "code = ?", code).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var count int64
	if err := a.db.WithContext(ctx).Model(&Trip{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return false, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return count > 0, nil
}

func (a *adapterGormPostgresql) ListByUserID(ctx context.Context, userID uuid.UUID, filter tripdomain.ListFilter) ([]domaintrip.Trip, int64, error) {
	query := a.db.WithContext(ctx).Model(&Trip{}).
		Joins("JOIN tripmate.trip_participants p ON p.trip_id = tripmate.trips.id AND p.deleted_at IS NULL").
		Where("p.user_id = ?", userID).
		Where("tripmate.trips.is_archived = ?", filter.Archived)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	var models []Trip
	if err := query.Order("tripmate.trips.start_date DESC").Limit(filter.PerPage).
		Offset((filter.Page - 1) * filter.PerPage).Find(&models).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := make([]domaintrip.Trip, len(models))
	for index, model := range models {
		result[index] = toDomain(model)
	}
	return result, total, nil
}

func (a *adapterGormPostgresql) Update(ctx context.Context, entity *domaintrip.Trip) (*domaintrip.Trip, error) {
	model := fromDomain(*entity)
	result := a.db.WithContext(ctx).Model(&Trip{}).Where("id = ? AND version = ?", entity.ID, entity.Version).Updates(map[string]any{
		"name": model.Name, "base_currency": model.BaseCurrency,
		"setting_edit_permission": model.EditPermission, "setting_approval_expenses": model.ApprovalExpenses,
		"setting_approval_settlements":        model.ApprovalSettlements,
		"setting_multi_currency_enabled":      model.MultiCurrency,
		"setting_allow_settlement_before_end": model.AllowSettlement,
		"version":                             gorm.Expr("version + 1"), "updated_at": gorm.Expr("now()"),
	})
	if result.Error != nil {
		return nil, apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return nil, apperror.New("CONCURRENT_MODIFICATION")
	}
	return a.GetByID(ctx, entity.ID)
}

func (a *adapterGormPostgresql) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if err := a.db.WithContext(ctx).Delete(&Trip{}, "id = ?", id).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func translate(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New("TRIP_NOT_FOUND")
	}
	return apperror.Wrap(err, "INTERNAL_ERROR")
}
