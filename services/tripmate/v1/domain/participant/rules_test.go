package participant

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type participantRulesRepo struct {
	membership    *domainparticipant.Participant
	membershipErr error
	target        *domainparticipant.Participant
	rows          []domainparticipant.Participant
	planners      int64
	created       *domainparticipant.Participant
	updated       *domainparticipant.Participant
	deleted       uuid.UUID
}

func (r *participantRulesRepo) Create(_ context.Context, value *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	r.created = value
	return value, nil
}
func (r *participantRulesRepo) GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	return r.membership, r.membershipErr
}
func (r *participantRulesRepo) GetByID(context.Context, uuid.UUID) (*domainparticipant.Participant, error) {
	return r.target, nil
}
func (r *participantRulesRepo) ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error) {
	return r.rows, nil
}
func (r *participantRulesRepo) Update(_ context.Context, value *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	r.updated = value
	return value, nil
}
func (r *participantRulesRepo) SoftDelete(_ context.Context, id uuid.UUID) error {
	r.deleted = id
	return nil
}
func (r *participantRulesRepo) CountPlanners(context.Context, uuid.UUID) (int64, error) {
	return r.planners, nil
}

type participantRulesTrips struct{ trip domaintrip.Trip }

func (r participantRulesTrips) GetByCode(context.Context, string) (*domaintrip.Trip, error) {
	copy := r.trip
	return &copy, nil
}

func TestJoinRejectsFinalizedTripAndDuplicateMembership(t *testing.T) {
	actor := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123", IsFinalized: true}
	repo := &participantRulesRepo{membershipErr: apperror.New("PARTICIPANT_NOT_FOUND")}
	service := NewService(repo, participantRulesTrips{trip: trip}, nil)

	if _, err := service.Join(context.Background(), actor, trip.Code); !apperror.Is(err, "TRIP_FINALIZED") {
		t.Fatalf("Join(finalized) error = %v", err)
	}
	trip.IsFinalized = false
	repo.membership = &domainparticipant.Participant{TripID: trip.ID, UserID: actor}
	repo.membershipErr = nil
	service = NewService(repo, participantRulesTrips{trip: trip}, nil)
	if _, err := service.Join(context.Background(), actor, trip.Code); !apperror.Is(err, "ALREADY_PARTICIPANT") {
		t.Fatalf("Join(duplicate) error = %v", err)
	}
}

func TestListMasksOtherAccountsButPreservesViewersAccount(t *testing.T) {
	viewer := uuid.New()
	other := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	membership := domainparticipant.Participant{TripID: trip.ID, UserID: viewer, Role: domainparticipant.RoleParticipant}
	repo := &participantRulesRepo{rows: []domainparticipant.Participant{
		{ID: uuid.New(), TripID: trip.ID, UserID: viewer, BankInfo: &domainparticipant.BankInfo{AccountNumber: "12345678"}},
		{ID: uuid.New(), TripID: trip.ID, UserID: other, BankInfo: &domainparticipant.BankInfo{AccountNumber: "87654321"}},
	}}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: membership})

	rows, err := NewService(repo, participantRulesTrips{}, nil).List(ctx, viewer, trip.Code)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rows[0].BankInfo.AccountNumber != "12345678" || rows[1].BankInfo.AccountNumber != "••••4321" {
		t.Fatalf("account numbers = %q, %q", rows[0].BankInfo.AccountNumber, rows[1].BankInfo.AccountNumber)
	}
}

func TestUpdateCannotDemoteLastPlanner(t *testing.T) {
	actor := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	membership := domainparticipant.Participant{ID: uuid.New(), TripID: trip.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	target := membership
	repo := &participantRulesRepo{target: &target, planners: 1}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: membership})
	nextRole := domainparticipant.RoleParticipant

	_, err := NewService(repo, participantRulesTrips{}, nil).Update(ctx, actor, trip.Code, target.ID, nil, &nextRole)
	if !apperror.Is(err, "VALIDATION_FAILED") || repo.updated != nil {
		t.Fatalf("Update() error = %v, updated = %+v", err, repo.updated)
	}
}

func TestAddRequiresPlanner(t *testing.T) {
	actor := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	member := domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RoleParticipant}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: member})
	_, err := NewService(&participantRulesRepo{}, participantRulesTrips{}, nil).Add(ctx, actor, trip.Code, uuid.New())
	if !apperror.Is(err, "PLANNER_ONLY") {
		t.Fatalf("Add(participant) error = %v", err)
	}
}

func TestUpdateBankInfoAllowsOnlyOwnerOrPlanner(t *testing.T) {
	actor, owner := uuid.New(), uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	target := &domainparticipant.Participant{ID: uuid.New(), TripID: trip.ID, UserID: owner, Role: domainparticipant.RoleParticipant}
	bank := &domainparticipant.BankInfo{BankName: "Bank", AccountNumber: "1234", AccountHolder: "Owner"}
	repo := &participantRulesRepo{target: target}
	member := domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RoleParticipant}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: member})
	if _, err := NewService(repo, participantRulesTrips{}, nil).Update(ctx, actor, trip.Code, target.ID, bank, nil); !apperror.Is(err, "FORBIDDEN") {
		t.Fatalf("Update(other participant) error = %v", err)
	}
	member.Role = domainparticipant.RolePlanner
	ctx = tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: member})
	if _, err := NewService(repo, participantRulesTrips{}, nil).Update(ctx, actor, trip.Code, target.ID, bank, nil); err != nil {
		t.Fatalf("Update(planner) error = %v", err)
	}
	target.UserID = actor
	member.Role = domainparticipant.RoleParticipant
	ctx = tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: member})
	if _, err := NewService(repo, participantRulesTrips{}, nil).Update(ctx, actor, trip.Code, target.ID, bank, nil); err != nil {
		t.Fatalf("Update(owner) error = %v", err)
	}
}

type activityState struct{ active bool }

func (s activityState) HasActivity(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return s.active, nil
}

func TestRemoveProtectsSelfAndParticipantsWithActivity(t *testing.T) {
	actor := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	planner := domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: planner})
	repo := &participantRulesRepo{target: &domainparticipant.Participant{ID: uuid.New(), TripID: trip.ID, UserID: actor}}
	if err := NewService(repo, participantRulesTrips{}, nil).Remove(ctx, actor, trip.Code, repo.target.ID); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("Remove(self) error = %v", err)
	}
	repo.target.UserID = uuid.New()
	if err := NewService(repo, participantRulesTrips{}, activityState{active: true}).Remove(ctx, actor, trip.Code, repo.target.ID); !apperror.Is(err, "PARTICIPANT_HAS_ACTIVITY") {
		t.Fatalf("Remove(active) error = %v", err)
	}
	if err := NewService(repo, participantRulesTrips{}, activityState{}).Remove(ctx, actor, trip.Code, repo.target.ID); err != nil || repo.deleted != repo.target.ID {
		t.Fatalf("Remove(inactive) error = %v, deleted = %s", err, repo.deleted)
	}
}
