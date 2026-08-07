package fx

import (
	"context"

	"github.com/google/uuid"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
)

type Repository interface {
	ListEffective(context.Context, uuid.UUID) ([]domainfx.Rate, error)
	Get(context.Context, *uuid.UUID, string, string) (*domainfx.Rate, error)
	Upsert(context.Context, domainfx.Rate) (*domainfx.Rate, error)
	ListGlobal(context.Context) ([]domainfx.Rate, error)
}
