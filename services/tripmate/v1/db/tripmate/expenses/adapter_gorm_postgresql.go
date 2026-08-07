package expenses

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	payersdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_payers"
	splitsdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/expense_splits"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

const expenseUserSelect = `tripmate.expenses.*,
	creator.email AS creator_email, creator.name AS creator_name, creator.avatar_url AS creator_avatar_url,
	creator.created_at AS creator_created_at, creator.updated_at AS creator_updated_at,
	approver.email AS approver_email, approver.name AS approver_name, approver.avatar_url AS approver_avatar_url,
	approver.created_at AS approver_created_at, approver.updated_at AS approver_updated_at`

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
	if err := a.withUsers(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{})).Select(expenseUserSelect).Where("tripmate.expenses.id = ?", id).First(&model).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	rows := []domainexpense.Expense{result}
	if err := a.hydrate(ctx, rows); err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (a *adapterGormPostgresql) ListByTripID(ctx context.Context, tripID uuid.UUID, filter expensedomain.Filter) ([]domainexpense.Expense, int64, expensedomain.Totals, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 || filter.PerPage > 100 {
		filter.PerPage = 20
	}
	where := []string{"e.trip_id = ?", "e.deleted_at IS NULL"}
	args := []any{tripID}
	if filter.PayerUserID != nil {
		where = append(where, "EXISTS (SELECT 1 FROM tripmate.expense_payers p WHERE p.expense_id = e.id AND p.user_id = ?)")
		args = append(args, *filter.PayerUserID)
	}
	if filter.SplitUserID != nil {
		where = append(where, "EXISTS (SELECT 1 FROM tripmate.expense_splits s WHERE s.expense_id = e.id AND s.user_id = ?)")
		args = append(args, *filter.SplitUserID)
	}
	if filter.Status != nil {
		where = append(where, "e.status = ?")
		args = append(args, *filter.Status)
	}
	if filter.Currency != "" {
		where = append(where, "e.currency = ?")
		args = append(args, strings.ToUpper(filter.Currency))
	}
	if filter.DateFrom != nil {
		where = append(where, "e.expense_date >= ?")
		args = append(args, *filter.DateFrom)
	}
	if filter.DateTo != nil {
		where = append(where, "e.expense_date <= ?")
		args = append(args, *filter.DateTo)
	}
	if filter.Query != "" {
		where = append(where, "e.description ILIKE ?")
		args = append(args, "%"+filter.Query+"%")
	}
	order := "expense_date DESC, created_at DESC"
	if filter.Sort == "amount_desc" {
		order = "amount DESC, expense_date DESC"
	}
	if filter.Sort == "created_desc" {
		order = "created_at DESC"
	}
	var models []Expense
	query := `WITH filtered AS (
		SELECT e.*, creator.email AS creator_email, creator.name AS creator_name,
			creator.avatar_url AS creator_avatar_url, creator.created_at AS creator_created_at,
			creator.updated_at AS creator_updated_at, approver.email AS approver_email,
			approver.name AS approver_name, approver.avatar_url AS approver_avatar_url,
			approver.created_at AS approver_created_at, approver.updated_at AS approver_updated_at
		FROM tripmate.expenses e
		JOIN tripmate.users creator ON creator.id = e.created_by_user_id
		LEFT JOIN tripmate.users approver ON approver.id = e.approved_by_user_id
		WHERE ` + strings.Join(where, " AND ") + `
	), windowed AS (
		SELECT filtered.*, count(*) OVER() AS total_count,
			sum(amount) OVER(PARTITION BY currency) AS currency_total,
			count(*) OVER(PARTITION BY status) AS status_count
		FROM filtered
	)
	SELECT windowed.*, jsonb_object_agg(currency, currency_total::text) OVER() AS currency_totals,
		jsonb_object_agg(status, status_count) OVER() AS status_totals
	FROM windowed ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, filter.PerPage, (filter.Page-1)*filter.PerPage)
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Raw(query, args...).Scan(&models).Error; err != nil {
		return nil, 0, expensedomain.Totals{}, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	var total int64
	totals := expensedomain.Totals{ByCurrency: map[string]decimal.Decimal{}, CountByStatus: map[domainexpense.Status]int64{}}
	if len(models) > 0 {
		total = models[0].TotalCount
		var currencyValues map[string]string
		var statusValues map[string]int64
		if err := json.Unmarshal(models[0].CurrencyTotals, &currencyValues); err != nil {
			return nil, 0, expensedomain.Totals{}, apperror.Wrap(err, "INTERNAL_ERROR")
		}
		if err := json.Unmarshal(models[0].StatusTotals, &statusValues); err != nil {
			return nil, 0, expensedomain.Totals{}, apperror.Wrap(err, "INTERNAL_ERROR")
		}
		for currency, raw := range currencyValues {
			value, err := decimal.NewFromString(raw)
			if err != nil {
				return nil, 0, expensedomain.Totals{}, apperror.Wrap(err, "INTERNAL_ERROR")
			}
			totals.ByCurrency[currency] = value
		}
		for status, count := range statusValues {
			totals.CountByStatus[domainexpense.Status(status)] = count
		}
	}
	rows := make([]domainexpense.Expense, len(models))
	for index, model := range models {
		rows[index] = toDomain(model)
	}
	if err := a.hydrate(ctx, rows); err != nil {
		return nil, 0, expensedomain.Totals{}, err
	}
	return rows, total, totals, nil
}

func (a *adapterGormPostgresql) ListApprovedByTripID(ctx context.Context, tripID uuid.UUID) ([]domainexpense.Expense, error) {
	var models []Expense
	query := a.withUsers(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{})).Select(`tripmate.expenses.*,
		creator.email AS creator_email, creator.name AS creator_name, creator.avatar_url AS creator_avatar_url,
		creator.created_at AS creator_created_at, creator.updated_at AS creator_updated_at,
		approver.email AS approver_email, approver.name AS approver_name, approver.avatar_url AS approver_avatar_url,
		approver.created_at AS approver_created_at, approver.updated_at AS approver_updated_at`)
	if err := query.Where("tripmate.expenses.trip_id = ? AND tripmate.expenses.status = ?", tripID, domainexpense.StatusApproved).Order("tripmate.expenses.expense_date, tripmate.expenses.id").Find(&models).Error; err != nil {
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

func (a *adapterGormPostgresql) ListForBalance(ctx context.Context, tripID uuid.UUID) ([]domainexpense.Expense, error) {
	var models []Expense
	query := a.withUsers(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Expense{})).Select(expenseUserSelect)
	if err := query.Where("tripmate.expenses.trip_id = ?", tripID).Order("tripmate.expenses.id").Find(&models).Error; err != nil {
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

func (a *adapterGormPostgresql) withUsers(query *gorm.DB) *gorm.DB {
	return query.Joins("JOIN tripmate.users AS creator ON creator.id = tripmate.expenses.created_by_user_id").
		Joins("LEFT JOIN tripmate.users AS approver ON approver.id = tripmate.expenses.approved_by_user_id")
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
