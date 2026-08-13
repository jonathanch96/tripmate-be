package invitation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	domaininv "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type invitationRepoStub struct {
	invitation *domaininv.Invitation
	created    *domaininv.Invitation
	listEmail  string
	listRows   []domaininv.Invitation
}

func (r *invitationRepoStub) Create(_ context.Context, invitation *domaininv.Invitation) (*domaininv.Invitation, error) {
	r.created = invitation
	return invitation, nil
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
func (r *invitationRepoStub) ListPendingByEmail(_ context.Context, email string) ([]domaininv.Invitation, error) {
	r.listEmail = email
	return r.listRows, nil
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
	addCode     string
	addedUser   uuid.UUID
}

func (s *invitationParticipantServiceStub) Join(_ context.Context, actor uuid.UUID, code string) (*domainparticipant.Participant, error) {
	s.joinCode = code
	s.participant.UserID = actor
	return &s.participant, nil
}
func (s *invitationParticipantServiceStub) Add(_ context.Context, _ uuid.UUID, code string, userID uuid.UUID) (*domainparticipant.Participant, error) {
	s.addCode = code
	s.addedUser = userID
	s.participant.UserID = userID
	return &s.participant, nil
}

type invitationUserFinder struct {
	user   *domainuser.User
	err    error
	member *domainuser.User
}

func (f invitationUserFinder) FindByEmail(context.Context, string) (*domainuser.User, error) {
	return f.user, f.err
}
func (f invitationUserFinder) CreateMember(_ context.Context, email, name, password string) (*domainuser.User, error) {
	if f.member != nil {
		return f.member, nil
	}
	return &domainuser.User{ID: uuid.New(), Email: email, Name: name, PasswordHash: "hashed:" + password}, nil
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

func TestAcceptRejectsExpiredInvitationWithoutJoining(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	repo := &invitationRepoStub{invitation: &domaininv.Invitation{
		ID: uuid.New(), TripID: trip.ID, Email: "invited@example.com", Token: "expired",
		Status: domaininv.StatusPending, ExpiresAt: now.Add(-time.Second),
	}}
	participants := &invitationParticipantServiceStub{}
	service := NewService(repo, invitationTripRepoStub{trip: trip}, nil, participants).(*service)
	service.clock = func() time.Time { return now }

	_, err := service.Accept(context.Background(), uuid.New(), "invited@example.com", "expired")
	if !apperror.Is(err, "INVITATION_NOT_FOUND") || participants.joinCode != "" {
		t.Fatalf("Accept() error = %v, join code = %q", err, participants.joinCode)
	}
}

func TestInviteExistingUserAddsParticipantImmediately(t *testing.T) {
	actor := uuid.New()
	user := &domainuser.User{ID: uuid.New(), Email: "member@example.com"}
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	planner := domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	participants := &invitationParticipantServiceStub{participant: domainparticipant.Participant{TripID: trip.ID}}
	service := NewService(
		&invitationRepoStub{},
		invitationTripRepoStub{trip: trip},
		invitationUserFinder{user: user},
		participants,
	)
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: planner})

	result, err := service.Invite(ctx, actor, trip.Code, " Member@Example.com ", "", "")
	if err != nil {
		t.Fatalf("Invite() error = %v", err)
	}
	if result.Status != "added" || participants.addCode != trip.Code || participants.addedUser != user.ID {
		t.Fatalf("Invite() = %+v, add code = %q, user = %s", result, participants.addCode, participants.addedUser)
	}
}

func TestInviteRequiresPlanner(t *testing.T) {
	actor := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	member := domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RoleParticipant}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: member})
	_, err := NewService(&invitationRepoStub{}, invitationTripRepoStub{trip: trip}, invitationUserFinder{}, &invitationParticipantServiceStub{}).Invite(ctx, actor, trip.Code, "friend@example.com", "", "Password1")
	if !apperror.Is(err, "PLANNER_ONLY") {
		t.Fatalf("Invite(participant) error = %v", err)
	}
}

// Adding someone with no account yet must not leave them un-assignable until they sign up - they
// become a real participant immediately, with the password the trip owner set for them right
// here. There is no pending/invited state and no invitation row is created.
func TestInviteWithNoExistingAccountCreatesTheAccountAndAddsParticipantImmediately(t *testing.T) {
	actor := uuid.New()
	trip := domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	planner := domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	member := &domainuser.User{ID: uuid.New(), Email: "friend@example.com"}
	repo := &invitationRepoStub{}
	participants := &invitationParticipantServiceStub{}
	service := NewService(
		repo, invitationTripRepoStub{trip: trip},
		invitationUserFinder{err: apperror.New("USER_NOT_FOUND"), member: member},
		participants,
	)
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: trip, Participant: planner})

	result, err := service.Invite(ctx, actor, trip.Code, " Friend@Example.com ", "Friend", "Password1")
	if err != nil {
		t.Fatalf("Invite() error = %v", err)
	}
	if participants.addCode != trip.Code || participants.addedUser != member.ID {
		t.Fatalf("participant was not added for the new member: add code = %q, user = %s", participants.addCode, participants.addedUser)
	}
	if result.Status != "created" || result.Participant == nil || result.Participant.UserID != member.ID {
		t.Fatalf("Invite() result = %+v", result)
	}
	if repo.created != nil {
		t.Fatalf("no invitation row should be created: %+v", repo.created)
	}
}

func TestListForMeNormalizesEmail(t *testing.T) {
	repo := &invitationRepoStub{listRows: []domaininv.Invitation{{ID: uuid.New()}}}
	service := NewService(repo, invitationTripRepoStub{}, invitationUserFinder{}, &invitationParticipantServiceStub{})
	rows, err := service.ListForMe(context.Background(), " PERSON@Example.COM ")
	if err != nil || len(rows) != 1 || repo.listEmail != "person@example.com" {
		t.Fatalf("ListForMe() = %+v, %v; email = %q", rows, err, repo.listEmail)
	}
}
