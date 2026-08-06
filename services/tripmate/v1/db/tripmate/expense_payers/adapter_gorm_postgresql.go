package expense_payers

import (
	"context"

	"github.com/google/uuid"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}
func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) BulkCreate(ctx context.Context, expenseID uuid.UUID, rows []domainexpense.Payer) error {
	if len(rows) == 0 {
		return nil
	}
	models := make([]Payer, len(rows))
	for index, row := range rows {
		row.ExpenseID = expenseID
		if row.ID == uuid.Nil {
			row.ID = uuid.New()
		}
		models[index] = fromDomain(row)
	}
	return appdb.FromContext(ctx, a.db).WithContext(ctx).Create(&models).Error
}

func (a *adapterGormPostgresql) ListByExpenseIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domainexpense.Payer, error) {
	result := make(map[uuid.UUID][]domainexpense.Payer, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var models []Payer
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Table("tripmate.expense_payers AS p").
		Select(`p.*, u.email AS user_email, u.name AS user_name, u.avatar_url AS user_avatar_url,
			u.created_at AS user_created_at, u.updated_at AS user_updated_at`).
		Joins("JOIN tripmate.users AS u ON u.id = p.user_id").
		Where("p.expense_id IN ?", ids).Order("p.created_at, p.id").Find(&models).Error; err != nil {
		return nil, err
	}
	for _, model := range models {
		result[model.ExpenseID] = append(result[model.ExpenseID], toDomain(model))
	}
	return result, nil
}

func (a *adapterGormPostgresql) DeleteByExpenseID(ctx context.Context, expenseID uuid.UUID) error {
	return appdb.FromContext(ctx, a.db).WithContext(ctx).Where("expense_id = ?", expenseID).Delete(&Payer{}).Error
}
