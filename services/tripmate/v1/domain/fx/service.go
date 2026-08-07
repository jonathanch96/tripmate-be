package fx

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
)

type service struct{ deps Dependencies }

func NewService(deps Dependencies) Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &service{deps}
}
func (s *service) EffectiveTable(ctx context.Context, tripID uuid.UUID) (*RateTable, error) {
	rows, err := s.deps.Repo.ListEffective(ctx, tripID)
	if err != nil {
		return nil, err
	}
	return NewRateTable(rows), nil
}
func (s *service) SetTripRate(ctx context.Context, _ identity.Identity, tc tripctx.TripContext, in SetRateInput) (*domainfx.Rate, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	in.FromCurrency = normalize(in.FromCurrency)
	in.ToCurrency = normalize(in.ToCurrency)
	if in.FromCurrency == in.ToCurrency || !in.Rate.IsPositive() {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	now := s.deps.Clock().UTC()
	return s.deps.Repo.Upsert(ctx, domainfx.Rate{ID: uuid.New(), TripID: &tc.Trip.ID, FromCurrency: in.FromCurrency, ToCurrency: in.ToCurrency, Rate: in.Rate, IsFinal: true, Source: domainfx.SourceManual, EffectiveAt: now, CreatedAt: now, UpdatedAt: now})
}
func (s *service) ListForTrip(ctx context.Context, tc tripctx.TripContext) ([]domainfx.Rate, error) {
	return s.deps.Repo.ListEffective(ctx, tc.Trip.ID)
}
func (s *service) ListGlobal(ctx context.Context) ([]domainfx.Rate, error) {
	return s.deps.Repo.ListGlobal(ctx)
}
func (s *service) LockAll(ctx context.Context, tripID uuid.UUID) error {
	rows, err := s.deps.Repo.ListEffective(ctx, tripID)
	if err != nil {
		return err
	}
	now := s.deps.Clock().UTC()
	for _, row := range rows {
		row.TripID = &tripID
		row.ID = uuid.New()
		row.IsFinal = true
		row.Source = domainfx.SourceManual
		row.EffectiveAt = now
		if _, err = s.deps.Repo.Upsert(ctx, row); err != nil {
			return err
		}
	}
	return nil
}
