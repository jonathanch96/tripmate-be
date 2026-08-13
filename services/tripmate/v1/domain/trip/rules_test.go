package trip

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type tripRepoFake struct {
	existing map[string]bool
	updated  *domaintrip.Trip
	trip     *domaintrip.Trip
}

func (*tripRepoFake) Create(context.Context, *domaintrip.Trip) (*domaintrip.Trip, error) {
	return nil, nil
}
func (r *tripRepoFake) GetByID(context.Context, uuid.UUID) (*domaintrip.Trip, error) {
	if r.trip == nil {
		return nil, apperror.New("TRIP_NOT_FOUND")
	}
	copy := *r.trip
	return &copy, nil
}
func (r *tripRepoFake) GetByCode(context.Context, string) (*domaintrip.Trip, error) {
	if r.trip == nil {
		return nil, apperror.New("TRIP_NOT_FOUND")
	}
	copy := *r.trip
	return &copy, nil
}
func (r *tripRepoFake) ExistsByCode(_ context.Context, code string) (bool, error) {
	return r.existing[code], nil
}
func (*tripRepoFake) ListByUserID(context.Context, uuid.UUID, ListFilter) ([]domaintrip.Trip, int64, error) {
	return nil, 0, nil
}
func (r *tripRepoFake) Update(_ context.Context, entity *domaintrip.Trip) (*domaintrip.Trip, error) {
	r.updated = entity
	return entity, nil
}
func (*tripRepoFake) SoftDelete(context.Context, uuid.UUID) error { return nil }
func (r *tripRepoFake) SetArchived(_ context.Context, _ uuid.UUID, archived bool) error {
	if r.trip != nil {
		r.trip.IsArchived = archived
	}
	return nil
}

type participantRepoFake struct {
	membership *domainparticipant.Participant
	err        error
}

func (participantRepoFake) Create(context.Context, *domainparticipant.Participant) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (r participantRepoFake) GetByTripAndUser(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	return r.membership, r.err
}

type transactionSpy struct {
	trip        *domaintrip.Trip
	participant *domainparticipant.Participant
}

func (s *transactionSpy) CreateTripWithPlanner(_ context.Context, trip *domaintrip.Trip, participant *domainparticipant.Participant) (*domaintrip.Trip, error) {
	s.trip, s.participant = trip, participant
	return trip, nil
}

type sequenceCodes struct {
	codes []string
	next  int
}

func (s *sequenceCodes) Generate() (string, error) {
	code := s.codes[s.next]
	s.next++
	return code, nil
}

type expenseState struct {
	count      int64
	currencies []string
}

func (s expenseState) CountByTrip(context.Context, uuid.UUID) (int64, error) { return s.count, nil }
func (s expenseState) CurrenciesByTrip(context.Context, uuid.UUID) ([]string, error) {
	return s.currencies, nil
}

func TestCreateRetriesCollisionsAndCreatesPlannerAtomically(t *testing.T) {
	actor := uuid.New()
	repo := &tripRepoFake{existing: map[string]bool{"TAKEN1": true}}
	tx := &transactionSpy{}
	codes := &sequenceCodes{codes: []string{"TAKEN1", "FREE01"}}
	service := NewService(Dependencies{Repo: repo, Participants: participantRepoFake{}, Tx: tx, Codes: codes})

	created, err := service.Create(context.Background(), actor, CreateInput{
		Name: "  Summer trip  ", BaseCurrency: "usd",
		StartDate: time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Code != "FREE01" || created.Name != "Summer trip" || created.BaseCurrency != "USD" {
		t.Fatalf("Create() = %+v", created)
	}
	if tx.participant == nil || tx.participant.UserID != actor || tx.participant.Role != domainparticipant.RolePlanner || tx.participant.TripID != created.ID {
		t.Fatalf("planner participant = %+v", tx.participant)
	}
}

func TestCreateStopsAfterFiveCodeCollisions(t *testing.T) {
	repo := &tripRepoFake{existing: map[string]bool{"TAKEN1": true}}
	codes := &sequenceCodes{codes: []string{"TAKEN1", "TAKEN1", "TAKEN1", "TAKEN1", "TAKEN1"}}
	service := NewService(Dependencies{Repo: repo, Participants: participantRepoFake{}, Tx: &transactionSpy{}, Codes: codes})

	_, err := service.Create(context.Background(), uuid.New(), CreateInput{
		Name: "Trip", BaseCurrency: "USD", StartDate: time.Now(), EndDate: time.Now(),
	})
	if !apperror.Is(err, "TRIP_CODE_COLLISION") || codes.next != 5 {
		t.Fatalf("Create() error = %v, attempts = %d", err, codes.next)
	}
}

func TestUpdateSettingsProtectsCurrenciesAlreadyInUse(t *testing.T) {
	actor := uuid.New()
	entity := domaintrip.Trip{
		ID: uuid.New(), Code: "ABC123", BaseCurrency: "USD",
		Settings: domaintrip.Settings{MultiCurrencyEnabled: true}, Version: 2,
	}
	membership := domainparticipant.Participant{TripID: entity.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: entity, Participant: membership})
	repo := &tripRepoFake{}
	service := NewService(Dependencies{Repo: repo, Expenses: expenseState{currencies: []string{"USD", "EUR"}}})

	_, err := service.UpdateSettings(ctx, actor, entity.Code, UpdateSettingsInput{
		Settings: domaintrip.Settings{MultiCurrencyEnabled: false}, Version: entity.Version,
	})
	if !apperror.Is(err, "VALIDATION_FAILED") || repo.updated != nil {
		t.Fatalf("UpdateSettings() error = %v, updated = %+v", err, repo.updated)
	}
}

func TestUpdateSettingsProtectsBaseCurrencyAfterFirstExpense(t *testing.T) {
	actor := uuid.New()
	entity := domaintrip.Trip{ID: uuid.New(), Code: "ABC123", BaseCurrency: "USD", Version: 2}
	membership := domainparticipant.Participant{TripID: entity.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: entity, Participant: membership})
	repo := &tripRepoFake{}
	service := NewService(Dependencies{Repo: repo, Expenses: expenseState{count: 1}})
	nextCurrency := "EUR"

	_, err := service.UpdateSettings(ctx, actor, entity.Code, UpdateSettingsInput{
		BaseCurrency: &nextCurrency, Settings: entity.Settings, Version: entity.Version,
	})
	if !apperror.Is(err, "VALIDATION_FAILED") || repo.updated != nil {
		t.Fatalf("UpdateSettings() error = %v, updated = %+v", err, repo.updated)
	}
}

func TestCreateRejectsInvalidCurrencyAndBackwardsDates(t *testing.T) {
	service := NewService(Dependencies{})
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	_, err := service.Create(context.Background(), uuid.New(), CreateInput{Name: "Trip", BaseCurrency: "XYZ", StartDate: now, EndDate: now})
	if !apperror.Is(err, "INVALID_CURRENCY") {
		t.Fatalf("invalid currency error = %v", err)
	}
	_, err = service.Create(context.Background(), uuid.New(), CreateInput{Name: "Trip", BaseCurrency: "USD", StartDate: now, EndDate: now.Add(-24 * time.Hour)})
	if !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("backwards dates error = %v", err)
	}
}

func TestGetByCodeRequiresMembership(t *testing.T) {
	actor := uuid.New()
	entity := &domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	repo := &tripRepoFake{trip: entity}
	service := NewService(Dependencies{Repo: repo, Participants: participantRepoFake{err: apperror.New("PARTICIPANT_NOT_FOUND")}})
	if _, err := service.GetByCode(context.Background(), actor, entity.Code); !apperror.Is(err, "NOT_TRIP_MEMBER") {
		t.Fatalf("GetByCode(non-member) error = %v", err)
	}
	member := &domainparticipant.Participant{TripID: entity.ID, UserID: actor}
	service = NewService(Dependencies{Repo: repo, Participants: participantRepoFake{membership: member}})
	if found, err := service.GetByCode(context.Background(), actor, entity.Code); err != nil || found.ID != entity.ID {
		t.Fatalf("GetByCode(member) = %+v, %v", found, err)
	}
}

func TestUpdateSettingsRequiresPlannerAndMutableTrip(t *testing.T) {
	actor := uuid.New()
	entity := domaintrip.Trip{ID: uuid.New(), Code: "ABC123", BaseCurrency: "USD", Version: 1}
	member := domainparticipant.Participant{TripID: entity.ID, UserID: actor, Role: domainparticipant.RoleParticipant}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: entity, Participant: member})
	service := NewService(Dependencies{})
	if _, err := service.UpdateSettings(ctx, actor, entity.Code, UpdateSettingsInput{}); !apperror.Is(err, "PLANNER_ONLY") {
		t.Fatalf("UpdateSettings(participant) error = %v", err)
	}
	entity.IsFinalized = true
	member.Role = domainparticipant.RolePlanner
	ctx = tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: entity, Participant: member})
	if _, err := service.UpdateSettings(ctx, actor, entity.Code, UpdateSettingsInput{}); !apperror.Is(err, "TRIP_FINALIZED") {
		t.Fatalf("UpdateSettings(finalized) error = %v", err)
	}
}

func TestArchiveRequiresPlannerAndTogglesState(t *testing.T) {
	actor := uuid.New()
	entity := domaintrip.Trip{ID: uuid.New(), Code: "ABC123", Version: 1}
	participantMember := domainparticipant.Participant{TripID: entity.ID, UserID: actor, Role: domainparticipant.RoleParticipant}
	ctx := tripctx.WithContext(context.Background(), tripctx.TripContext{Trip: entity, Participant: participantMember})
	guard := NewService(Dependencies{})
	if _, err := guard.Archive(ctx, actor, entity.Code); !apperror.Is(err, "PLANNER_ONLY") {
		t.Fatalf("Archive(participant) error = %v", err)
	}

	planner := domainparticipant.Participant{TripID: entity.ID, UserID: actor, Role: domainparticipant.RolePlanner}
	repo := &tripRepoFake{trip: &entity}
	service := NewService(Dependencies{Repo: repo, Participants: participantRepoFake{membership: &planner}})

	archived, err := service.Archive(context.Background(), actor, entity.Code)
	if err != nil || !archived.IsArchived {
		t.Fatalf("Archive() = %+v, err = %v", archived, err)
	}
	if _, err := service.Archive(context.Background(), actor, entity.Code); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("Archive(already archived) error = %v", err)
	}

	restored, err := service.Unarchive(context.Background(), actor, entity.Code)
	if err != nil || restored.IsArchived {
		t.Fatalf("Unarchive() = %+v, err = %v", restored, err)
	}
	if _, err := service.Unarchive(context.Background(), actor, entity.Code); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("Unarchive(already active) error = %v", err)
	}
}
