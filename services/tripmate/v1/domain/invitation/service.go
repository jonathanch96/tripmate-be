package invitation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
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

type Repository interface {
	Create(context.Context, *domaininv.Invitation) (*domaininv.Invitation, error)
	Update(context.Context, *domaininv.Invitation) (*domaininv.Invitation, error)
	GetByToken(context.Context, string) (*domaininv.Invitation, error)
	GetPending(context.Context, uuid.UUID, string) (*domaininv.Invitation, error)
	ListPendingByEmail(context.Context, string) ([]domaininv.Invitation, error)
	ListByTrip(context.Context, uuid.UUID) ([]domaininv.Invitation, error)
	Revoke(context.Context, uuid.UUID) error
}
type TripRepository interface {
	GetByID(context.Context, uuid.UUID) (*domaintrip.Trip, error)
	GetByCode(context.Context, string) (*domaintrip.Trip, error)
}
type UserFinder interface {
	FindByEmail(context.Context, string) (*domainuser.User, error)
	CreateInvited(ctx context.Context, email, password string) (*domainuser.User, error)
}
type Result struct {
	Status      string
	Invitation  *domaininv.Invitation
	Participant *domainparticipant.Participant
}
type Service interface {
	Invite(context.Context, uuid.UUID, string, string, string) (*Result, error)
	Accept(context.Context, uuid.UUID, string, string) (*domainparticipant.Participant, error)
	ListForMe(context.Context, string) ([]domaininv.Invitation, error)
	ListTrip(context.Context, uuid.UUID, string) ([]domaininv.Invitation, error)
	Revoke(context.Context, uuid.UUID, string, uuid.UUID) error
}
type service struct {
	repo  Repository
	trips TripRepository
	users UserFinder
	parts participantdomain.Service
	clock func() time.Time
}

func NewService(repo Repository, trips TripRepository, users UserFinder, parts participantdomain.Service) Service {
	return &service{repo: repo, trips: trips, users: users, parts: parts, clock: time.Now}
}
func (s *service) Invite(ctx context.Context, actor uuid.UUID, code, email, password string) (*Result, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	t, err := s.plannerTrip(ctx, actor, code)
	if err != nil {
		return nil, err
	}
	if u, e := s.users.FindByEmail(ctx, email); e == nil {
		p, e := s.parts.Add(ctx, actor, code, u.ID)
		return &Result{Status: "added", Participant: p}, e
	} else if !apperror.Is(e, "USER_NOT_FOUND") {
		return nil, e
	}
	// No account exists for this email yet - create one now, with the password the inviter chose,
	// instead of waiting for them to sign up. This lets the planner assign them as an expense
	// payer/split participant right away, and lets the invitee sign in immediately using the
	// credentials shared alongside the invite link. They're still tracked with a pending
	// invitation/token so Accept() can mark it accepted once they do sign in.
	invited, err := s.users.CreateInvited(ctx, email, password)
	if err != nil {
		return nil, err
	}
	participant, err := s.parts.Add(ctx, actor, code, invited.ID)
	if err != nil {
		return nil, err
	}
	if existing, e := s.repo.GetPending(ctx, t.ID, email); e == nil {
		existing.ExpiresAt = s.clock().UTC().Add(14 * 24 * time.Hour)
		updated, e := s.repo.Update(ctx, existing)
		return &Result{Status: "invited", Invitation: updated, Participant: participant}, e
	} else if !apperror.Is(e, "INVITATION_NOT_FOUND") {
		return nil, e
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	inv := &domaininv.Invitation{ID: uuid.New(), TripID: t.ID, Email: email, Token: hex.EncodeToString(raw), Status: domaininv.StatusPending, InvitedBy: actor, ExpiresAt: s.clock().UTC().Add(14 * 24 * time.Hour)}
	created, err := s.repo.Create(ctx, inv)
	return &Result{Status: "invited", Invitation: created, Participant: participant}, err
}
func (s *service) Accept(ctx context.Context, actor uuid.UUID, email, token string) (*domainparticipant.Participant, error) {
	inv, err := s.repo.GetByToken(ctx, token)
	if err != nil || inv.Status != domaininv.StatusPending || !inv.ExpiresAt.After(s.clock()) {
		return nil, apperror.New("INVITATION_NOT_FOUND")
	}
	if !strings.EqualFold(inv.Email, email) {
		slog.InfoContext(ctx, "invitation accepted with different email", "invited_email", inv.Email, "actor_email", email)
	}
	t, err := s.trips.GetByID(ctx, inv.TripID)
	if err != nil {
		return nil, err
	}
	p, err := s.parts.Join(ctx, actor, t.Code)
	if apperror.Is(err, "ALREADY_PARTICIPANT") {
		p, err = s.parts.GetMembership(ctx, t.ID, actor)
	}
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	inv.Status = domaininv.StatusAccepted
	inv.AcceptedAt = &now
	if _, err = s.repo.Update(ctx, inv); err != nil {
		return nil, err
	}
	return p, nil
}
func (s *service) ListForMe(ctx context.Context, email string) ([]domaininv.Invitation, error) {
	return s.repo.ListPendingByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}
func (s *service) ListTrip(ctx context.Context, actor uuid.UUID, code string) ([]domaininv.Invitation, error) {
	t, err := s.plannerTrip(ctx, actor, code)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByTrip(ctx, t.ID)
}
func (s *service) Revoke(ctx context.Context, actor uuid.UUID, code string, id uuid.UUID) error {
	if _, err := s.ListTrip(ctx, actor, code); err != nil {
		return err
	}
	return s.repo.Revoke(ctx, id)
}
func (s *service) plannerTrip(ctx context.Context, actor uuid.UUID, code string) (*domaintrip.Trip, error) {
	if authorized, ok := tripctx.ForActor(ctx, actor, code); ok {
		if authorized.Participant.Role != domainparticipant.RolePlanner {
			return nil, apperror.New("PLANNER_ONLY")
		}
		return &authorized.Trip, nil
	}
	t, err := s.trips.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	p, err := s.parts.GetMembership(ctx, t.ID, actor)
	if err != nil || p.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	return t, nil
}
