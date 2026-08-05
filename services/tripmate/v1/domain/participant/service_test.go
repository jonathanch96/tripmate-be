package participant

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type participantRepoSpy struct {
	membershipLookups int
	rows              []domainparticipant.Participant
}

func (*participantRepoSpy) Create(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	panic("unexpected Create")
}
func (r *participantRepoSpy) GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	r.membershipLookups++
	return nil, nil
}
func (*participantRepoSpy) GetByID(context.Context, uuid.UUID) (*domainparticipant.Participant, error) {
	panic("unexpected GetByID")
}
func (r *participantRepoSpy) ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error) {
	return r.rows, nil
}
func (*participantRepoSpy) Update(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	panic("unexpected Update")
}
func (*participantRepoSpy) SoftDelete(context.Context, uuid.UUID) error {
	panic("unexpected SoftDelete")
}
func (*participantRepoSpy) CountPlanners(context.Context, uuid.UUID) (int64, error) {
	panic("unexpected CountPlanners")
}

type participantTripRepoSpy struct {
	lookups int
	trip    domaintrip.Trip
}

func (r *participantTripRepoSpy) GetByCode(context.Context, string) (*domaintrip.Trip, error) {
	r.lookups++
	copy := r.trip
	return &copy, nil
}

func TestListConsumesAuthorizedTripContextWithoutRepeatingGuardLookups(t *testing.T) {
	viewer := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	membership := domainparticipant.Participant{TripID: trip.ID, UserID: viewer, Role: domainparticipant.RoleParticipant}
	repo := &participantRepoSpy{rows: []domainparticipant.Participant{membership}}
	trips := &participantTripRepoSpy{trip: trip}
	service := NewService(repo, trips, nil)
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: membership})

	rows, err := service.List(ctx, viewer, trip.Code)
	if err != nil || len(rows) != 1 {
		t.Fatalf("List() = %+v, %v", rows, err)
	}
	if trips.lookups != 0 || repo.membershipLookups != 0 {
		t.Fatalf("guard lookups repeated: trip=%d membership=%d", trips.lookups, repo.membershipLookups)
	}
}
