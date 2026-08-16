package fx_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/shopspring/decimal"
)

type pair struct{ from, to string }

type repoStub struct {
	stored  []domainfx.Rate
	deleted []pair
}

func (r *repoStub) ListEffective(context.Context, uuid.UUID) ([]domainfx.Rate, error) {
	return r.stored, nil
}
func (r *repoStub) Get(context.Context, *uuid.UUID, string, string) (*domainfx.Rate, error) {
	return nil, nil
}

// Rows are keyed the way the real unique indexes are: trip-scoped and global rates for the same
// direction are separate rows.
func sameSlot(a, b domainfx.Rate) bool {
	if (a.TripID == nil) != (b.TripID == nil) {
		return false
	}
	if a.TripID != nil && *a.TripID != *b.TripID {
		return false
	}
	return a.FromCurrency == b.FromCurrency && a.ToCurrency == b.ToCurrency
}

func (r *repoStub) Upsert(_ context.Context, row domainfx.Rate) (*domainfx.Rate, error) {
	for i, existing := range r.stored {
		if sameSlot(existing, row) {
			r.stored[i] = row
			return &row, nil
		}
	}
	r.stored = append(r.stored, row)
	return &row, nil
}
func (r *repoStub) DeleteTripPair(_ context.Context, tripID uuid.UUID, from, to string) error {
	r.deleted = append(r.deleted, pair{from: from, to: to})
	kept := r.stored[:0]
	for _, row := range r.stored {
		if row.TripID != nil && *row.TripID == tripID && row.FromCurrency == from && row.ToCurrency == to {
			continue
		}
		kept = append(kept, row)
	}
	r.stored = kept
	return nil
}

func (r *repoStub) tripRows() []domainfx.Rate {
	var rows []domainfx.Rate
	for _, row := range r.stored {
		if row.TripID != nil {
			rows = append(rows, row)
		}
	}
	return rows
}
func (r *repoStub) ListGlobal(context.Context) ([]domainfx.Rate, error) { return r.stored, nil }

type directUOW struct{}

func (directUOW) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func rateFixture(stored ...domainfx.Rate) (fxdomain.Service, *repoStub, identity.Identity, tripctx.TripContext) {
	repo := &repoStub{stored: stored}
	actor := identity.Identity{UserID: uuid.New()}
	tc := tripctx.TripContext{
		Trip:        domaintrip.Trip{ID: uuid.New()},
		Participant: domainparticipant.Participant{UserID: actor.UserID, Role: domainparticipant.RolePlanner},
	}
	service := fxdomain.NewService(fxdomain.Dependencies{
		Repo:  repo,
		UOW:   directUOW{},
		Clock: func() time.Time { return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC) },
	})
	return service, repo, actor, tc
}

func setRate(t *testing.T, service fxdomain.Service, actor identity.Identity, tc tripctx.TripContext, from, to, rate string) {
	t.Helper()
	if _, err := service.SetTripRate(context.Background(), actor, tc, fxdomain.SetRateInput{
		FromCurrency: from, ToCurrency: to, Rate: decimal.RequireFromString(rate),
	}); err != nil {
		t.Fatalf("SetTripRate(%s→%s) error = %v", from, to, err)
	}
}

// Storing both directions of a pair lets them contradict each other, so setting one direction
// must retire the other rather than sit beside it.
func TestSetTripRateReplacesTheOppositeDirection(t *testing.T) {
	service, repo, actor, tc := rateFixture()

	setRate(t, service, actor, tc, "IDR", "PHP", "0.00333")
	setRate(t, service, actor, tc, "PHP", "IDR", "300")

	if len(repo.stored) != 1 {
		t.Fatalf("stored = %+v; want a single direction for the pair", repo.stored)
	}
	saved := repo.stored[0]
	if saved.FromCurrency != "PHP" || saved.ToCurrency != "IDR" || !saved.Rate.Equal(decimal.RequireFromString("300")) {
		t.Fatalf("stored = %+v; want the direction that was submitted last", saved)
	}

	// The surviving row still answers both directions through the rate table.
	table := fxdomain.NewRateTable(repo.stored)
	back, err := table.Convert(decimal.RequireFromString("300"), "IDR", "PHP")
	if err != nil || !back.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("Convert(300 IDR→PHP) = %s, %v; want 1", back, err)
	}
}

func TestSetTripRateNormalizesBeforeMatchingTheOppositeDirection(t *testing.T) {
	service, repo, actor, tc := rateFixture()

	setRate(t, service, actor, tc, "usd", "php", "56.5")
	setRate(t, service, actor, tc, " PHP ", "usd", "0.0177")

	if len(repo.stored) != 1 {
		t.Fatalf("stored = %+v; want casing and padding to resolve to one pair", repo.stored)
	}
	if got := repo.deleted[1]; got.from != "USD" || got.to != "PHP" {
		t.Fatalf("deleted = %+v; want the normalized opposite direction", got)
	}
}

func TestSetTripRateLeavesUnrelatedPairsAlone(t *testing.T) {
	service, repo, actor, tc := rateFixture()

	setRate(t, service, actor, tc, "USD", "PHP", "56.5")
	setRate(t, service, actor, tc, "EUR", "PHP", "61")

	if len(repo.stored) != 2 {
		t.Fatalf("stored = %+v; want both pairs kept", repo.stored)
	}
}

func TestLockAllKeepsOneDirectionPerPair(t *testing.T) {
	tripID := uuid.New()
	service, repo, _, _ := rateFixture(
		domainfx.Rate{FromCurrency: "IDR", ToCurrency: "PHP", Rate: decimal.RequireFromString("0.00333")},
		domainfx.Rate{FromCurrency: "PHP", ToCurrency: "IDR", Rate: decimal.RequireFromString("300")},
	)

	if err := service.LockAll(context.Background(), tripID); err != nil {
		t.Fatal(err)
	}
	locked := repo.tripRows()
	if len(locked) != 1 {
		t.Fatalf("trip rows = %+v; want locking to collapse the pair to one direction", locked)
	}
	if !locked[0].IsFinal {
		t.Fatalf("trip row = %+v; want the locked row marked final", locked[0])
	}
}
