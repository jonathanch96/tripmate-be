package trip_invitations

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}

func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) Create(ctx context.Context, entity *domaininvitation.Invitation) (*domaininvitation.Invitation, error) {
	model := fromDomain(*entity)
	if err := a.db.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) Update(ctx context.Context, entity *domaininvitation.Invitation) (*domaininvitation.Invitation, error) {
	model := fromDomain(*entity)
	if err := a.db.WithContext(ctx).Model(&Invitation{}).Where("id = ?", entity.ID).Updates(map[string]any{
		"status": model.Status, "expires_at": model.ExpiresAt, "accepted_at": model.AcceptedAt,
		"updated_at": gorm.Expr("now()"),
	}).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return a.GetByToken(ctx, entity.Token)
}

func (a *adapterGormPostgresql) GetByToken(ctx context.Context, token string) (*domaininvitation.Invitation, error) {
	var model Invitation
	if err := a.db.WithContext(ctx).First(&model, "token = ?", token).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetPending(ctx context.Context, tripID uuid.UUID, email string) (*domaininvitation.Invitation, error) {
	var model Invitation
	if err := a.db.WithContext(ctx).First(&model, "trip_id = ? AND email = ? AND status = 'pending'", tripID, email).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) ListPendingByEmail(ctx context.Context, email string) ([]domaininvitation.Invitation, error) {
	return a.list(ctx, "email = ? AND status = 'pending' AND expires_at > now()", email)
}

func (a *adapterGormPostgresql) ListByTrip(ctx context.Context, tripID uuid.UUID) ([]domaininvitation.Invitation, error) {
	return a.list(ctx, "trip_id = ?", tripID)
}

func (a *adapterGormPostgresql) list(ctx context.Context, query string, arg any) ([]domaininvitation.Invitation, error) {
	var models []Invitation
	if err := a.db.WithContext(ctx).Where(query, arg).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := make([]domaininvitation.Invitation, len(models))
	for index, model := range models {
		result[index] = toDomain(model)
	}
	return result, nil
}

func (a *adapterGormPostgresql) Revoke(ctx context.Context, id uuid.UUID) error {
	result := a.db.WithContext(ctx).Model(&Invitation{}).Where("id = ? AND status = 'pending'", id).Update("status", "revoked")
	if result.Error != nil {
		return apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	if result.RowsAffected == 0 {
		return apperror.New("INVITATION_NOT_FOUND")
	}
	return nil
}

func translate(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New("INVITATION_NOT_FOUND")
	}
	return apperror.Wrap(err, "INTERNAL_ERROR")
}
