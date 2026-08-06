package expenses

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	payersdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_payers"
	splitsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_splits"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}
func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) Create(ctx context.Context, entity *domainexpense.Expense) (*domainexpense.Expense, error) {
	model := fromDomain(*entity)
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetByID(ctx context.Context, id uuid.UUID) (*domainexpense.Expense, error) {
	var model Expense
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	rows := []domainexpense.Expense{result}
	if err := a.hydrate(ctx, rows); err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (a *adapterGormPostgresql) ListByTripID(ctx context.Context, tripID uuid.UUID, filter expensedomain.Filter) ([]domainexpense.Expense, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 || filter.PerPage > 100 {
		filter.PerPage = 20
	}
	db := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{}).Where("trip_id = ?", tripID)
	if filter.PayerUserID != nil {
		db = db.Where("id IN (SELECT expense_id FROM tripmate.expense_payers WHERE user_id = ?)", *filter.PayerUserID)
	}
	if filter.SplitUserID != nil {
		db = db.Where("id IN (SELECT expense_id FROM tripmate.expense_splits WHERE user_id = ?)", *filter.SplitUserID)
	}
	if filter.Status != nil {
		db = db.Where("status = ?", *filter.Status)
	}
	if filter.Currency != "" {
		db = db.Where("currency = ?", strings.ToUpper(filter.Currency))
	}
	if filter.DateFrom != nil {
		db = db.Where("expense_date >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		db = db.Where("expense_date <= ?", *filter.DateTo)
	}
	if filter.Query != "" {
		db = db.Where("description ILIKE ?", "%"+filter.Query+"%")
	}
	order := "expense_date DESC, created_at DESC"
	if filter.Sort == "amount_desc" {
		order = "amount DESC, expense_date DESC"
	}
	if filter.Sort == "created_desc" {
		order = "created_at DESC"
	}
	var models []Expense
	if err := db.Select("tripmate.expenses.*, count(*) OVER() AS total_count").Order(order).Offset((filter.Page - 1) * filter.PerPage).Limit(filter.PerPage).Find(&models).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	var total int64
	if len(models) > 0 {
		total = models[0].TotalCount
	}
	rows := make([]domainexpense.Expense, len(models))
	for index, model := range models {
		rows[index] = toDomain(model)
	}
	if err := a.hydrate(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (a *adapterGormPostgresql) ListApprovedByTripID(ctx context.Context, tripID uuid.UUID) ([]domainexpense.Expense, error) {
	var models []Expense
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Where("trip_id = ? AND status = ?", tripID, domainexpense.StatusApproved).Order("expense_date, id").Find(&models).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	rows := make([]domainexpense.Expense, len(models))
	for index, model := range models {
		rows[index] = toDomain(model)
	}
	if err := a.hydrate(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (a *adapterGormPostgresql) hydrate(ctx context.Context, rows []domainexpense.Expense) error {
	ids := make([]uuid.UUID, len(rows))
	for index := range rows {
		ids[index] = rows[index].ID
	}
	payers, err := payersdb.New(appdb.FromContext(ctx, a.db)).ListByExpenseIDs(ctx, ids)
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	splits, err := splitsdb.New(appdb.FromContext(ctx, a.db)).ListByExpenseIDs(ctx, ids)
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	for index := range rows {
		rows[index].Payers = payers[rows[index].ID]
		rows[index].Splits = splits[rows[index].ID]
	}
	return nil
}

func (a *adapterGormPostgresql) Update(ctx context.Context, entity *domainexpense.Expense) (*domainexpense.Expense, error) {
	model := fromDomain(*entity)
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{}).Where("id = ? AND version = ?", entity.ID, entity.Version).Updates(map[string]any{
		"expense_date": model.ExpenseDate, "description": model.Description, "amount": model.Amount,
		"currency": model.Currency, "split_type": model.SplitType, "status": model.Status, "source": model.Source,
		"note": model.Note, "approved_by_user_id": model.ApprovedByUserID, "approved_at": model.ApprovedAt,
		"rejected_reason": model.RejectedReason, "version": gorm.Expr("version + 1"), "updated_at": gorm.Expr("now()"),
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
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Delete(&Expense{}, "id = ?", id)
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("EXPENSE_NOT_FOUND")
	}
	return nil
}

func (a *adapterGormPostgresql) CountByTripAndUser(ctx context.Context, tripID, userID uuid.UUID) (int64, error) {
	var count int64
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).Raw(`SELECT count(DISTINCT e.id) FROM tripmate.expenses e
		WHERE e.trip_id = ? AND e.deleted_at IS NULL AND (e.created_by_user_id = ? OR EXISTS
		(SELECT 1 FROM tripmate.expense_payers p WHERE p.expense_id=e.id AND p.user_id=?) OR EXISTS
		(SELECT 1 FROM tripmate.expense_splits s WHERE s.expense_id=e.id AND s.user_id=?))`, tripID, userID, userID, userID).Scan(&count).Error
	return count, err
}

func (a *adapterGormPostgresql) DistinctCurrencies(ctx context.Context, tripID uuid.UUID) ([]string, error) {
	var currencies []string
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{}).Distinct("currency").Where("trip_id = ?", tripID).Order("currency").Pluck("currency", &currencies).Error
	return currencies, err
}

// CountByTrip and CurrenciesByTrip implement the trip-domain read ports used
// when trip settings are changed.
func (a *adapterGormPostgresql) CountByTrip(ctx context.Context, tripID uuid.UUID) (int64, error) {
	var count int64
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{}).Where("trip_id = ?", tripID).Count(&count).Error
	return count, err
}

func (a *adapterGormPostgresql) CurrenciesByTrip(ctx context.Context, tripID uuid.UUID) ([]string, error) {
	return a.DistinctCurrencies(ctx, tripID)
}

// HasActivity implements participant.ActivityCounter.
func (a *adapterGormPostgresql) HasActivity(ctx context.Context, tripID, userID uuid.UUID) (bool, error) {
	count, err := a.CountByTripAndUser(ctx, tripID, userID)
	return count > 0, err
}

func translate(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New("EXPENSE_NOT_FOUND")
	}
	return apperror.Wrap(err, "INTERNAL_ERROR")
}
