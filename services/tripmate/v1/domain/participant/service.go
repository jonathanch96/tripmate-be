package participant

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type Repository interface {
	Create(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error)
	GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error)
	GetByID(context.Context, uuid.UUID) (*domainparticipant.Participant, error)
	ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error)
	Update(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error)
	SoftDelete(context.Context, uuid.UUID) error
	CountPlanners(context.Context, uuid.UUID) (int64, error)
}
type TripRepository interface {
	GetByCode(context.Context, string) (*domaintrip.Trip, error)
}
type ActivityCounter interface {
	HasActivity(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}
type NoopActivityCounter struct{}

func (NoopActivityCounter) HasActivity(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

type Service interface {
	Join(context.Context, uuid.UUID, string) (*domainparticipant.Participant, error)
	Add(context.Context, uuid.UUID, string, uuid.UUID) (*domainparticipant.Participant, error)
	List(context.Context, uuid.UUID, string) ([]domainparticipant.Participant, error)
	Update(context.Context, uuid.UUID, string, uuid.UUID, *domainparticipant.BankInfo, *domainparticipant.Role, *string) (*domainparticipant.Participant, error)
	Remove(context.Context, uuid.UUID, string, uuid.UUID) error
	GetMembership(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error)
}
type service struct {
	repo     Repository
	trips    TripRepository
	activity ActivityCounter
}

func NewService(repo Repository, trips TripRepository, activity ActivityCounter) Service {
	if activity == nil {
		activity = NoopActivityCounter{}
	}
	return &service{repo, trips, activity}
}
func (s *service) GetMembership(ctx context.Context, tripID, userID uuid.UUID) (*domainparticipant.Participant, error) {
	return s.repo.GetByTripAndUser(ctx, tripID, userID)
}
func (s *service) Join(ctx context.Context, actor uuid.UUID, code string) (*domainparticipant.Participant, error) {
	t, err := s.trips.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if err = t.AssertMutable(); err != nil {
		return nil, err
	}
	if _, err = s.repo.GetByTripAndUser(ctx, t.ID, actor); err == nil {
		return nil, apperror.New("ALREADY_PARTICIPANT")
	} else if !apperror.Is(err, "PARTICIPANT_NOT_FOUND") {
		return nil, err
	}
	return s.repo.Create(ctx, &domainparticipant.Participant{ID: uuid.New(), TripID: t.ID, UserID: actor, Role: domainparticipant.RoleParticipant, JoinedAt: time.Now().UTC()})
}
func (s *service) Add(ctx context.Context, actor uuid.UUID, code string, userID uuid.UUID) (*domainparticipant.Participant, error) {
	t, err := s.planner(ctx, actor, code)
	if err != nil {
		return nil, err
	}
	if err = t.AssertMutable(); err != nil {
		return nil, err
	}
	if _, err = s.repo.GetByTripAndUser(ctx, t.ID, userID); err == nil {
		return nil, apperror.New("ALREADY_PARTICIPANT")
	} else if !apperror.Is(err, "PARTICIPANT_NOT_FOUND") {
		return nil, err
	}
	return s.repo.Create(ctx, &domainparticipant.Participant{ID: uuid.New(), TripID: t.ID, UserID: userID, Role: domainparticipant.RoleParticipant, JoinedAt: time.Now().UTC()})
}
func (s *service) List(ctx context.Context, viewer uuid.UUID, code string) ([]domainparticipant.Participant, error) {
	t, _, err := s.memberTrip(ctx, viewer, code)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListByTripID(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].BankInfo != nil && viewer != rows[i].UserID {
			copy := *rows[i].BankInfo
			copy.AccountNumber = domainparticipant.MaskAccount(copy.AccountNumber)
			rows[i].BankInfo = &copy
		}
	}
	return rows, nil
}
func (s *service) Update(ctx context.Context, actor uuid.UUID, code string, id uuid.UUID, bank *domainparticipant.BankInfo, role *domainparticipant.Role, displayName *string) (*domainparticipant.Participant, error) {
	t, member, err := s.memberTrip(ctx, actor, code)
	if err != nil {
		return nil, err
	}
	if err = t.AssertMutable(); err != nil {
		return nil, err
	}
	target, err := s.repo.GetByID(ctx, id)
	if err != nil || target.TripID != t.ID {
		return nil, apperror.New("PARTICIPANT_NOT_FOUND")
	}
	if bank != nil && actor != target.UserID && member.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("FORBIDDEN")
	}
	if displayName != nil && actor != target.UserID && member.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("FORBIDDEN")
	}
	if role != nil {
		if member.Role != domainparticipant.RolePlanner {
			return nil, apperror.New("PLANNER_ONLY")
		}
		if target.Role == domainparticipant.RolePlanner && *role != domainparticipant.RolePlanner {
			count, e := s.repo.CountPlanners(ctx, t.ID)
			if e != nil {
				return nil, e
			}
			if count <= 1 {
				return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "role", Rule: "last_planner", Message: "a trip must have at least one planner"}})
			}
		}
		target.Role = *role
	}
	if bank != nil {
		bank.BankName = strings.TrimSpace(bank.BankName)
		target.BankInfo = bank
	}
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if trimmed == "" {
			target.DisplayName = nil
		} else {
			target.DisplayName = &trimmed
		}
	}
	return s.repo.Update(ctx, target)
}
func (s *service) Remove(ctx context.Context, actor uuid.UUID, code string, id uuid.UUID) error {
	t, err := s.planner(ctx, actor, code)
	if err != nil {
		return err
	}
	if err = t.AssertMutable(); err != nil {
		return err
	}
	target, err := s.repo.GetByID(ctx, id)
	if err != nil || target.TripID != t.ID {
		return apperror.New("PARTICIPANT_NOT_FOUND")
	}
	if target.UserID == actor {
		return apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "participant", Rule: "self", Message: "planner cannot remove themselves"}})
	}
	active, err := s.activity.HasActivity(ctx, t.ID, target.UserID)
	if err != nil {
		return err
	}
	if active {
		return apperror.New("PARTICIPANT_HAS_ACTIVITY")
	}
	return s.repo.SoftDelete(ctx, id)
}
func (s *service) planner(ctx context.Context, actor uuid.UUID, code string) (*domaintrip.Trip, error) {
	t, p, err := s.memberTrip(ctx, actor, code)
	if err != nil {
		return nil, err
	}
	if p.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	return t, nil
}

func (s *service) memberTrip(ctx context.Context, actor uuid.UUID, code string) (*domaintrip.Trip, *domainparticipant.Participant, error) {
	if authorized, ok := tripctx.ForActor(ctx, actor, code); ok {
		return &authorized.Trip, &authorized.Participant, nil
	}
	t, err := s.trips.GetByCode(ctx, code)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.repo.GetByTripAndUser(ctx, t.ID, actor)
	if err != nil {
		return nil, nil, apperror.New("NOT_TRIP_MEMBER")
	}
	return t, p, nil
}
