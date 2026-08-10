package expense_categories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}
func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

// ListForTrip returns the global defaults plus this trip's custom categories, defaults first.
func (a *adapterGormPostgresql) ListForTrip(ctx context.Context, tripID uuid.UUID) ([]domaincat.ExpenseCategory, error) {
	var models []ExpenseCategory
	if err := a.db.WithContext(ctx).Where("trip_id IS NULL OR trip_id = ?", tripID).
		Order("is_default DESC, name").Find(&models).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := make([]domaincat.ExpenseCategory, len(models))
	for index, model := range models {
		result[index] = toDomain(model)
	}
	return result, nil
}

func (a *adapterGormPostgresql) GetByID(ctx context.Context, id uuid.UUID) (*domaincat.ExpenseCategory, error) {
	var model ExpenseCategory
	if err := a.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) ExistsByName(ctx context.Context, tripID *uuid.UUID, name string) (bool, error) {
	var count int64
	query := a.db.WithContext(ctx).Model(&ExpenseCategory{}).Where("lower(name) = ?", strings.ToLower(name))
	if tripID == nil {
		query = query.Where("trip_id IS NULL")
	} else {
		query = query.Where("trip_id = ?", *tripID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return count > 0, nil
}

func (a *adapterGormPostgresql) Create(ctx context.Context, entity *domaincat.ExpenseCategory) (*domaincat.ExpenseCategory, error) {
	model := fromDomain(*entity)
	if err := a.db.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) Delete(ctx context.Context, id uuid.UUID) error {
	result := a.db.WithContext(ctx).Where("id = ?", id).Delete(&ExpenseCategory{})
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("EXPENSE_CATEGORY_NOT_FOUND")
	}
	return nil
}

func translate(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New("EXPENSE_CATEGORY_NOT_FOUND")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apperror.New("EXPENSE_CATEGORY_ALREADY_EXISTS")
	}
	return apperror.Wrap(err, "INTERNAL_ERROR")
}
