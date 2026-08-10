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
	// A currency pair is one slot, not two. RateTable already reads a stored A→B backwards by
	// dividing, so keeping an explicit B→A row alongside it lets the two directions drift into
	// contradiction (IDR→PHP 300 next to PHP→IDR 300 round-trips 1 IDR into 90,000 IDR). The
	// direction the planner just submitted wins and the opposite row is dropped.
	var saved *domainfx.Rate
	err := s.deps.UOW.Do(ctx, func(ctx context.Context) error {
		if err := s.deps.Repo.DeleteTripPair(ctx, tc.Trip.ID, in.ToCurrency, in.FromCurrency); err != nil {
			return err
		}
		row, err := s.deps.Repo.Upsert(ctx, domainfx.Rate{ID: uuid.New(), TripID: &tc.Trip.ID, FromCurrency: in.FromCurrency, ToCurrency: in.ToCurrency, Rate: in.Rate, IsFinal: true, Source: domainfx.SourceManual, EffectiveAt: now, CreatedAt: now, UpdatedAt: now})
		saved = row
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// DeleteTripRate removes one trip-scoped exchange rate row (e.g. dropping a currency the trip no
// longer uses). It only ever removes the exact direction given - SetTripRate is what keeps a pair
// down to one direction, so callers should pass whichever direction ListForTrip returned.
func (s *service) DeleteTripRate(ctx context.Context, _ identity.Identity, tc tripctx.TripContext, from, to string) error {
	if err := tc.Trip.AssertMutable(); err != nil {
		return err
	}
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return apperror.New("PLANNER_ONLY")
	}
	from, to = normalize(from), normalize(to)
	if from == "" || to == "" {
		return apperror.New("VALIDATION_FAILED")
	}
	return s.deps.Repo.DeleteTripPair(ctx, tc.Trip.ID, from, to)
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
	// Global rows may carry both directions of a pair; locking them verbatim would recreate the
	// contradiction SetTripRate prevents, so only the first direction seen is pinned to the trip.
	locked := make(map[pairKey]struct{}, len(rows))
	for _, row := range rows {
		from, to := normalize(row.FromCurrency), normalize(row.ToCurrency)
		if _, done := locked[pairKey{from: to, to: from}]; done {
			continue
		}
		locked[pairKey{from: from, to: to}] = struct{}{}
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
