package trip_participants

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}

func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) Create(ctx context.Context, entity *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	model := fromDomain(*entity)
	if err := a.db.WithContext(ctx).Create(&model).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperror.New("ALREADY_PARTICIPANT")
		}
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetByTripAndUser(ctx context.Context, tripID, userID uuid.UUID) (*domainparticipant.Participant, error) {
	var model Participant
	if err := a.db.WithContext(ctx).First(&model, "trip_id = ? AND user_id = ?", tripID, userID).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetByID(ctx context.Context, id uuid.UUID) (*domainparticipant.Participant, error) {
	var model Participant
	if err := a.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err)
	}
	result := toDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) ListByTripID(ctx context.Context, tripID uuid.UUID) ([]domainparticipant.Participant, error) {
	var rows []participantWithUser
	if err := appdb.FromContext(ctx, a.db).WithContext(ctx).Table("tripmate.trip_participants p").
		Select("p.*, u.email, u.name, u.avatar_url, u.created_at user_created_at, u.updated_at user_updated_at").
		Joins("JOIN tripmate.users u ON u.id = p.user_id AND u.deleted_at IS NULL").
		Where("p.trip_id = ? AND p.deleted_at IS NULL", tripID).Order("p.joined_at").Scan(&rows).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := make([]domainparticipant.Participant, len(rows))
	for index, row := range rows {
		result[index] = participantRowToDomain(row)
	}
	return result, nil
}

func (a *adapterGormPostgresql) Update(ctx context.Context, entity *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	model := fromDomain(*entity)
	if err := a.db.WithContext(ctx).Model(&Participant{}).Where("id = ?", entity.ID).Updates(map[string]any{
		"role": model.Role, "bank_name": model.BankName, "bank_account_number": model.BankAccountNumber,
		"bank_account_holder": model.BankAccountHolder, "updated_at": gorm.Expr("now()"),
	}).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return a.GetByID(ctx, entity.ID)
}

func (a *adapterGormPostgresql) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return wrap(a.db.WithContext(ctx).Delete(&Participant{}, "id = ?", id).Error)
}

func (a *adapterGormPostgresql) CountPlanners(ctx context.Context, tripID uuid.UUID) (int64, error) {
	var count int64
	err := a.db.WithContext(ctx).Model(&Participant{}).Where("trip_id = ? AND role = 'planner'", tripID).Count(&count).Error
	return count, wrap(err)
}

func translate(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New("PARTICIPANT_NOT_FOUND")
	}
	return apperror.Wrap(err, "INTERNAL_ERROR")
}

func wrap(err error) error {
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}
