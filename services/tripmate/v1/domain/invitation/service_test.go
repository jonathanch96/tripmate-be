package invitation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	domaininv "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type invitationRepoStub struct{ invitation *domaininv.Invitation }

func (r *invitationRepoStub) Create(context.Context, *domaininv.Invitation) (*domaininv.Invitation, error) {
	panic("unexpected Create")
}
func (r *invitationRepoStub) Update(_ context.Context, invitation *domaininv.Invitation) (*domaininv.Invitation, error) {
	r.invitation = invitation
	return invitation, nil
}
func (r *invitationRepoStub) GetByToken(context.Context, string) (*domaininv.Invitation, error) {
	copy := *r.invitation
	return &copy, nil
}
func (r *invitationRepoStub) GetPending(context.Context, uuid.UUID, string) (*domaininv.Invitation, error) {
	return nil, apperror.New("INVITATION_NOT_FOUND")
}
func (r *invitationRepoStub) ListPendingByEmail(context.Context, string) ([]domaininv.Invitation, error) {
	return nil, nil
}
func (r *invitationRepoStub) ListByTrip(context.Context, uuid.UUID) ([]domaininv.Invitation, error) {
	return nil, nil
}
func (r *invitationRepoStub) Revoke(context.Context, uuid.UUID) error { return nil }

type invitationTripRepoStub struct{ trip domaintrip.Trip }

func (r invitationTripRepoStub) GetByID(_ context.Context, id uuid.UUID) (*domaintrip.Trip, error) {
	if id != r.trip.ID {
		return nil, apperror.New("TRIP_NOT_FOUND")
	}
	copy := r.trip
	return &copy, nil
}
func (r invitationTripRepoStub) GetByCode(_ context.Context, code string) (*domaintrip.Trip, error) {
	if code != r.trip.Code {
		return nil, apperror.New("TRIP_NOT_FOUND")
	}
	copy := r.trip
	return &copy, nil
}

type invitationParticipantServiceStub struct {
	participant domainparticipant.Participant
	joinCode    string
}

func (s *invitationParticipantServiceStub) Join(_ context.Context, actor uuid.UUID, code string) (*domainparticipant.Participant, error) {
	s.joinCode = code
	s.participant.UserID = actor
	return &s.participant, nil
}
func (*invitationParticipantServiceStub) Add(context.Context, uuid.UUID, string, uuid.UUID) (*domainparticipant.Participant, error) {
	panic("unexpected Add")
}
func (*invitationParticipantServiceStub) List(context.Context, uuid.UUID, string) ([]domainparticipant.Participant, error) {
	panic("unexpected List")
}
func (*invitationParticipantServiceStub) Update(context.Context, uuid.UUID, string, uuid.UUID, *domainparticipant.BankInfo, *domainparticipant.Role) (*domainparticipant.Participant, error) {
	panic("unexpected Update")
}
func (*invitationParticipantServiceStub) Remove(context.Context, uuid.UUID, string, uuid.UUID) error {
	panic("unexpected Remove")
}
func (s *invitationParticipantServiceStub) GetMembership(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	return &s.participant, nil
}

var _ participantdomain.Service = (*invitationParticipantServiceStub)(nil)

func TestAcceptResolvesInvitationTripThroughDeclaredRepositoryContract(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	actor := uuid.New()
	repo := &invitationRepoStub{invitation: &domaininv.Invitation{
		ID: uuid.New(), TripID: trip.ID, Email: "invited@example.com", Token: "token",
		Status: domaininv.StatusPending, ExpiresAt: now.Add(time.Hour),
	}}
	participants := &invitationParticipantServiceStub{participant: domainparticipant.Participant{TripID: trip.ID}}
	service := NewService(repo, invitationTripRepoStub{trip: trip}, nil, participants).(*service)
	service.clock = func() time.Time { return now }

	participant, err := service.Accept(context.Background(), actor, "actor@example.com", "token")
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	if participant.UserID != actor || participants.joinCode != trip.Code {
		t.Fatalf("Accept() participant = %+v, join code = %q", participant, participants.joinCode)
	}
	if repo.invitation.Status != domaininv.StatusAccepted || repo.invitation.AcceptedAt == nil {
		t.Fatalf("invitation was not accepted: %+v", repo.invitation)
	}
}
