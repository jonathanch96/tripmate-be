package settlements

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func New(db *gorm.DB) *adapterGormPostgresql { return &adapterGormPostgresql{db} }

const selectUsers = `tripmate.settlements.*, from_user.email from_email, from_user.name from_name, from_user.avatar_url from_avatar_url, to_user.email to_email, to_user.name to_name, to_user.avatar_url to_avatar_url`

func (a *adapterGormPostgresql) withUsers(q *gorm.DB) *gorm.DB {
	return q.Joins("JOIN tripmate.users from_user ON from_user.id=tripmate.settlements.from_user_id").Joins("JOIN tripmate.users to_user ON to_user.id=tripmate.settlements.to_user_id")
}
func (a *adapterGormPostgresql) Create(ctx context.Context, e *domainsettlement.Settlement) (*domainsettlement.Settlement, error) {
	m := fromDomain(*e)
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Create(&m).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return a.GetByID(ctx, m.ID)
}
func (a *adapterGormPostgresql) GetByID(ctx context.Context, id uuid.UUID) (*domainsettlement.Settlement, error) {
	var m Settlement
	err := a.withUsers(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Settlement{})).Select(selectUsers).Where("tripmate.settlements.id=?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperror.New("SETTLEMENT_NOT_FOUND")
	}
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	e := toDomain(m)
	return &e, nil
}
func (a *adapterGormPostgresql) Update(ctx context.Context, e *domainsettlement.Settlement) (*domainsettlement.Settlement, error) {
	m := fromDomain(*e)
	r := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Settlement{}).Where("id=? AND version=?", e.ID, e.Version).Updates(map[string]any{
		"amount": m.Amount, "currency": m.Currency, "method": m.Method,
		"bank_name": m.BankName, "bank_account_number": m.BankAccountNumber, "bank_account_holder": m.BankAccountHolder,
		"note": m.Note, "proof_url": m.ProofURL, "settlement_date": m.SettlementDate,
		"status": m.Status, "approved_by_user_id": m.ApprovedByUserID, "approved_at": m.ApprovedAt, "rejected_reason": m.RejectedReason,
		"version": gorm.Expr("version+1"), "updated_at": gorm.Expr("now()"),
	})
	if r.Error != nil {
		return nil, apperror.Wrap(r.Error, "INTERNAL_ERROR")
	}
	if r.RowsAffected == 0 {
		return nil, apperror.New("CONCURRENT_MODIFICATION")
	}
	return a.GetByID(ctx, e.ID)
}
func (a *adapterGormPostgresql) ListByTripID(ctx context.Context, tripID uuid.UUID, f settlementdomain.Filter) ([]domainsettlement.Settlement, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 100 {
		f.PerPage = 20
	}
	q := a.withUsers(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Settlement{})).Where("tripmate.settlements.trip_id=?", tripID)
	if f.Status != nil {
		q = q.Where("tripmate.settlements.status=?", *f.Status)
	}
	if f.ParticipantID != nil {
		q = q.Where("(from_user_id=? OR to_user_id=?)", *f.ParticipantID, *f.ParticipantID)
	}
	if f.Method != nil {
		q = q.Where("method=?", *f.Method)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	var ms []Settlement
	if err := q.Select(selectUsers).Order("tripmate.settlements.created_at DESC, tripmate.settlements.id").Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage).Find(&ms).Error; err != nil {
		return nil, 0, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	rows := make([]domainsettlement.Settlement, len(ms))
	for i := range ms {
		rows[i] = toDomain(ms[i])
	}
	return rows, total, nil
}

// listAll reads every matching settlement without pagination. Balance calculation must see the
// whole history of a trip, so it must never share the paginated ListByTripID read path.
func (a *adapterGormPostgresql) listAll(ctx context.Context, tripID uuid.UUID, status *domainsettlement.Status) ([]domainsettlement.Settlement, error) {
	q := a.withUsers(appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Settlement{})).Where("tripmate.settlements.trip_id=?", tripID)
	if status != nil {
		q = q.Where("tripmate.settlements.status=?", *status)
	}
	var ms []Settlement
	if err := q.Select(selectUsers).Order("tripmate.settlements.created_at, tripmate.settlements.id").Find(&ms).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	rows := make([]domainsettlement.Settlement, len(ms))
	for i := range ms {
		rows[i] = toDomain(ms[i])
	}
	return rows, nil
}
func (a *adapterGormPostgresql) ListApprovedByTripID(ctx context.Context, tripID uuid.UUID) ([]domainsettlement.Settlement, error) {
	status := domainsettlement.StatusApproved
	return a.listAll(ctx, tripID, &status)
}
func (a *adapterGormPostgresql) ListForBalance(ctx context.Context, tripID uuid.UUID) ([]domainsettlement.Settlement, error) {
	return a.listAll(ctx, tripID, nil)
}
func (a *adapterGormPostgresql) SoftDelete(ctx context.Context, id uuid.UUID) error {
	r := appdb.FromContext(ctx, a.db).WithContext(ctx).Delete(&Settlement{}, "id=?", id)
	if r.Error != nil {
		return apperror.Wrap(r.Error, "INTERNAL_ERROR")
	}
	if r.RowsAffected == 0 {
		return apperror.New("SETTLEMENT_NOT_FOUND")
	}
	return nil
}

// HasActivity implements participant.ActivityCounter.
func (a *adapterGormPostgresql) HasActivity(ctx context.Context, tripID, userID uuid.UUID) (bool, error) {
	var count int64
	err := appdb.FromContext(ctx, a.db).WithContext(ctx).Model(&Settlement{}).
		Where("trip_id = ? AND (from_user_id = ? OR to_user_id = ?)", tripID, userID, userID).
		Count(&count).Error
	if err != nil {
		return false, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return count > 0, nil
}
