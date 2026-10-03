package oauth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appdb "github.com/jblabs/tripmate-be/services/tripmate/v1/db"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type adapterGormPostgresql struct{ db *gorm.DB }

func NewGormPostgresqlAdapter(db *gorm.DB) *adapterGormPostgresql {
	return &adapterGormPostgresql{db: db}
}

func New(db *gorm.DB) *adapterGormPostgresql { return NewGormPostgresqlAdapter(db) }

func (a *adapterGormPostgresql) conn(ctx context.Context) *gorm.DB {
	return appdb.FromContext(ctx, a.db).WithContext(ctx)
}

func (a *adapterGormPostgresql) CreateClient(ctx context.Context, entity *domainoauth.Client) (*domainoauth.Client, error) {
	model := clientFromDomain(*entity)
	if err := a.conn(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := clientToDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) GetClientByClientID(ctx context.Context, clientID string) (*domainoauth.Client, error) {
	var model Client
	if err := a.conn(ctx).First(&model, "client_id = ?", clientID).Error; err != nil {
		return nil, translate(err, "OAUTH_INVALID_CLIENT")
	}
	result := clientToDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) CreateRequest(ctx context.Context, entity *domainoauth.AuthRequest) (*domainoauth.AuthRequest, error) {
	model := requestFromDomain(*entity)
	if err := a.conn(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := requestToDomain(model, nil)
	return &result, nil
}

func (a *adapterGormPostgresql) GetRequest(ctx context.Context, id uuid.UUID) (*domainoauth.AuthRequest, error) {
	var model AuthRequest
	if err := a.conn(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err, "OAUTH_REQUEST_NOT_FOUND")
	}
	client, err := a.clientByID(ctx, model.ClientID)
	if err != nil {
		return nil, err
	}
	result := requestToDomain(model, client)
	return &result, nil
}

func (a *adapterGormPostgresql) GetRequestByCodeHash(ctx context.Context, hash string) (*domainoauth.AuthRequest, error) {
	var model AuthRequest
	if err := a.conn(ctx).First(&model, "code_hash = ?", hash).Error; err != nil {
		return nil, translate(err, "OAUTH_INVALID_GRANT")
	}
	result := requestToDomain(model, nil)
	return &result, nil
}

func (a *adapterGormPostgresql) UpdateRequestStatus(ctx context.Context, entity *domainoauth.AuthRequest, from domainoauth.RequestStatus) (bool, error) {
	result := a.conn(ctx).Model(&AuthRequest{}).Where("id = ? AND status = ?", entity.ID, string(from)).Updates(map[string]any{
		"status": string(entity.Status), "user_id": entity.UserID, "granted_scopes": pq.StringArray(entity.GrantedScopes),
		"code_hash": entity.CodeHash, "code_expires_at": entity.CodeExpiresAt,
	})
	if result.Error != nil {
		return false, apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	return result.RowsAffected == 1, nil
}

func (a *adapterGormPostgresql) GetActiveGrant(ctx context.Context, userID, clientID uuid.UUID) (*domainoauth.Grant, error) {
	var model Grant
	if err := a.conn(ctx).First(&model, "user_id = ? AND client_id = ? AND revoked_at IS NULL", userID, clientID).Error; err != nil {
		return nil, translate(err, "OAUTH_GRANT_NOT_FOUND")
	}
	client, err := a.clientByID(ctx, model.ClientID)
	if err != nil {
		return nil, err
	}
	result := grantToDomain(model, client)
	return &result, nil
}

func (a *adapterGormPostgresql) GetGrant(ctx context.Context, id uuid.UUID) (*domainoauth.Grant, error) {
	var model Grant
	if err := a.conn(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err, "OAUTH_GRANT_NOT_FOUND")
	}
	client, err := a.clientByID(ctx, model.ClientID)
	if err != nil {
		return nil, err
	}
	result := grantToDomain(model, client)
	return &result, nil
}

func (a *adapterGormPostgresql) CreateGrant(ctx context.Context, entity *domainoauth.Grant) (*domainoauth.Grant, error) {
	model := grantFromDomain(*entity)
	if err := a.conn(ctx).Create(&model).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	result := grantToDomain(model, nil)
	return &result, nil
}

func (a *adapterGormPostgresql) UpdateGrantScopes(ctx context.Context, id uuid.UUID, scopes []string) error {
	if err := a.conn(ctx).Model(&Grant{}).Where("id = ?", id).Update("scopes", pq.StringArray(scopes)).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) TouchGrant(ctx context.Context, id uuid.UUID, at time.Time) error {
	if err := a.conn(ctx).Model(&Grant{}).Where("id = ?", id).Update("last_used_at", at).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) RevokeGrant(ctx context.Context, id uuid.UUID) error {
	if err := a.conn(ctx).Model(&Grant{}).Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", gorm.Expr("now()")).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) ListActiveGrants(ctx context.Context, userID uuid.UUID) ([]domainoauth.Grant, error) {
	var models []Grant
	if err := a.conn(ctx).Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("last_used_at DESC").Find(&models).Error; err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	clientIDs := make([]uuid.UUID, 0, len(models))
	for _, model := range models {
		clientIDs = append(clientIDs, model.ClientID)
	}
	var clients []Client
	if len(clientIDs) > 0 {
		if err := a.conn(ctx).Where("id IN ?", clientIDs).Find(&clients).Error; err != nil {
			return nil, apperror.Wrap(err, "INTERNAL_ERROR")
		}
	}
	byID := make(map[uuid.UUID]*Client, len(clients))
	for index := range clients {
		byID[clients[index].ID] = &clients[index]
	}
	result := make([]domainoauth.Grant, len(models))
	for index, model := range models {
		result[index] = grantToDomain(model, byID[model.ClientID])
	}
	return result, nil
}

func (a *adapterGormPostgresql) CreateToken(ctx context.Context, entity *domainoauth.Token) error {
	model := tokenFromDomain(*entity)
	if err := a.conn(ctx).Create(&model).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) GetTokenByHash(ctx context.Context, hash string) (*domainoauth.Token, error) {
	var model Token
	if err := a.conn(ctx).First(&model, "token_hash = ?", hash).Error; err != nil {
		return nil, translate(err, "OAUTH_INVALID_TOKEN")
	}
	result := tokenToDomain(model)
	return &result, nil
}

func (a *adapterGormPostgresql) RevokeToken(ctx context.Context, id uuid.UUID, at time.Time) (bool, error) {
	result := a.conn(ctx).Model(&Token{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at)
	if result.Error != nil {
		return false, apperror.Wrap(result.Error, "INTERNAL_ERROR")
	}
	return result.RowsAffected == 1, nil
}

func (a *adapterGormPostgresql) RevokeTokensForGrant(ctx context.Context, grantID uuid.UUID) error {
	if err := a.conn(ctx).Model(&Token{}).Where("grant_id = ? AND revoked_at IS NULL", grantID).
		Update("revoked_at", gorm.Expr("now()")).Error; err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (a *adapterGormPostgresql) clientByID(ctx context.Context, id uuid.UUID) (*Client, error) {
	var model Client
	if err := a.conn(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, translate(err, "OAUTH_INVALID_CLIENT")
	}
	return &model, nil
}

func translate(err error, notFound string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New(notFound)
	}
	return apperror.Wrap(err, "INTERNAL_ERROR")
}
