package receipts

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func New(db *gorm.DB) *adapterGormPostgresql { return &adapterGormPostgresql{db} }

const selectUploader = `tripmate.receipts.*, uploader.email uploader_email, uploader.name uploader_name, uploader.avatar_url uploader_avatar_url`

func (a *adapterGormPostgresql) withUploader(q *gorm.DB) *gorm.DB {
	return q.Joins("JOIN tripmate.users uploader ON uploader.id = tripmate.receipts.uploaded_by")
}

func (a *adapterGormPostgresql) Create(ctx context.Context, e *domainreceipt.Receipt) (*domainreceipt.Receipt, error) {
	model := fromDomain(*e)
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return a.GetByID(ctx, model.ID)
}

// GetByID returns the receipt with its lines and whoever claimed each one. Three queries, never one
// per line: the items come in a single read and the assignments in another.
func (a *adapterGormPostgresql) GetByID(ctx context.Context, id uuid.UUID) (*domainreceipt.Receipt, error) {
	db := appdb.FromContext(ctx, a.db)
	var model Receipt
	err := a.withUploader(db.WithContext(ctx).Model(&Receipt{})).Select(selectUploader).
		Where("tripmate.receipts.id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	entity := toDomain(model)
	if entity.Items, err = a.itemsFor(ctx, id); err != nil {
		return nil, err
	}
	return &entity, nil
}

func (a *adapterGormPostgresql) itemsFor(ctx context.Context, receiptID uuid.UUID) ([]domainreceipt.Item, error) {
	db := appdb.FromContext(ctx, a.db)
	var models []Item
	if err := db.WithContext(ctx).Where("receipt_id = ?", receiptID).Order("sort_order, id").Find(&models).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	if len(models) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(models))
	for index, model := range models {
		ids[index] = model.ID
	}
	var assignments []Assignment
	if err := db.WithContext(ctx).Where("receipt_item_id IN ?", ids).Order("receipt_item_id, user_id").Find(&assignments).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	byItem := make(map[uuid.UUID][]uuid.UUID, len(models))
	for _, assignment := range assignments {
		byItem[assignment.ReceiptItemID] = append(byItem[assignment.ReceiptItemID], assignment.UserID)
	}
	rows := make([]domainreceipt.Item, len(models))
	for index, model := range models {
		rows[index] = itemToDomain(model)
		rows[index].Assignees = byItem[model.ID]
	}
	return rows, nil
}

func (a *adapterGormPostgresql) Update(ctx context.Context, e *domainreceipt.Receipt) (*domainreceipt.Receipt, error) {
	model := fromDomain(*e)
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Receipt{}).Where("id = ?", e.ID).
		Updates(map[string]any{
			"expense_id": model.ExpenseID, "merchant": model.Merchant, "receipt_date": model.ReceiptDate,
			"currency": model.Currency, "subtotal": model.Subtotal, "tax": model.Tax,
			"service_charge": model.ServiceCharge, "total": model.Total, "provider": model.Provider,
			"provider_model": model.ProviderModel, "raw_response": model.RawResponse,
			"status": model.Status, "failure_reason": model.FailureReason, "updated_at": gorm.Expr("now()"),
		})
	if result.Error != nil {
		return nil, apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return a.GetByID(ctx, e.ID)
}

func (a *adapterGormPostgresql) ListByTripID(ctx context.Context, tripID uuid.UUID, f receiptdomain.Filter) ([]domainreceipt.Receipt, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 100 {
		f.PerPage = 20
	}
	query := a.withUploader(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Receipt{})).
		Where("tripmate.receipts.trip_id = ?", tripID)
	if f.Status != nil {
		query = query.Where("tripmate.receipts.status = ?", string(*f.Status))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	var models []Receipt
	if err := query.Select(selectUploader).Order("tripmate.receipts.created_at DESC, tripmate.receipts.id").
		Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage).Find(&models).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	rows := make([]domainreceipt.Receipt, len(models))
	for index := range models {
		rows[index] = toDomain(models[index])
	}
	return rows, total, nil
}

func (a *adapterGormPostgresql) SoftDelete(ctx context.Context, id uuid.UUID) error {
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Delete(&Receipt{}, "id = ?", id)
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("RECEIPT_NOT_FOUND")
	}
	return nil
}

// ReplaceItems swaps the whole set of lines. Deleting the old rows takes their assignments with
// them through the cascade, which is what re-extraction means.
func (a *adapterGormPostgresql) ReplaceItems(ctx context.Context, receiptID uuid.UUID, items []domainreceipt.Item) error {
	db := appdb.FromContext(ctx, a.db).WithContext(ctx)
	if err := db.Where("receipt_id = ?", receiptID).Delete(&Item{}).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	if len(items) == 0 {
		return nil
	}
	models := make([]Item, len(items))
	for index, item := range items {
		models[index] = itemFromDomain(item)
		models[index].ReceiptID = receiptID
		if models[index].ID == uuid.Nil {
			models[index].ID = uuid.New()
		}
	}
	if err := db.Create(&models).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) CreateItem(ctx context.Context, e *domainreceipt.Item) (*domainreceipt.Item, error) {
	model := itemFromDomain(*e)
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return a.GetItem(ctx, model.ID)
}

func (a *adapterGormPostgresql) GetItem(ctx context.Context, id uuid.UUID) (*domainreceipt.Item, error) {
	db := appdb.FromContext(ctx, a.db)
	var model Item
	err := db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	entity := itemToDomain(model)
	var assignments []Assignment
	if err = db.WithContext(ctx).Where("receipt_item_id = ?", id).Order("user_id").Find(&assignments).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	for _, assignment := range assignments {
		entity.Assignees = append(entity.Assignees, assignment.UserID)
	}
	return &entity, nil
}

func (a *adapterGormPostgresql) UpdateItem(ctx context.Context, e *domainreceipt.Item) (*domainreceipt.Item, error) {
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Item{}).Where("id = ?", e.ID).
		Updates(map[string]any{"name": e.Name, "quantity": e.Quantity, "unit_price": e.UnitPrice,
			"total_price": e.TotalPrice, "sort_order": e.SortOrder, "updated_at": gorm.Expr("now()")})
	if result.Error != nil {
		return nil, apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return a.GetItem(ctx, e.ID)
}

func (a *adapterGormPostgresql) DeleteItem(ctx context.Context, id uuid.UUID) error {
	result := appdb.FromContext(ctx, a.db).WithContext(ctx).Delete(&Item{}, "id = ?", id)
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("RECEIPT_NOT_FOUND")
	}
	return nil
}

// ReplaceAssignments rewrites who is on the hook for every line of one receipt in a single pass
// (rule A-4: the set is replaced atomically, not merged).
func (a *adapterGormPostgresql) ReplaceAssignments(ctx context.Context, receiptID uuid.UUID, byItem map[uuid.UUID][]uuid.UUID) error {
	db := appdb.FromContext(ctx, a.db).WithContext(ctx)
	if err := db.Exec(`DELETE FROM tripmate.receipt_item_assignments
		WHERE receipt_item_id IN (SELECT id FROM tripmate.receipt_items WHERE receipt_id = ?)`, receiptID).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	rows := make([]Assignment, 0, len(byItem))
	now := time.Now().UTC()
	for itemID, users := range byItem {
		for _, userID := range users {
			rows = append(rows, Assignment{ID: uuid.New(), ReceiptItemID: itemID, UserID: userID, CreatedAt: now})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := db.Create(&rows).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

// ClearExpenseLink is rule C-7: when the expense a receipt produced is deleted, the receipt goes
// back to being convertible rather than pointing at a row that is gone.
func (a *adapterGormPostgresql) ClearExpenseLink(ctx context.Context, expenseID uuid.UUID) error {
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Receipt{}).
		Where("expense_id = ?", expenseID).
		Updates(map[string]any{"expense_id": nil, "status": string(domainreceipt.StatusExtracted), "updated_at": gorm.Expr("now()")}).Error
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}
