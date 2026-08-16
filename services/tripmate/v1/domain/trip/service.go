package trip

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

const codeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

type CryptoCodeGenerator struct{}

func (CryptoCodeGenerator) Generate() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i := range raw {
		raw[i] = codeAlphabet[int(raw[i])%len(codeAlphabet)]
	}
	return string(raw), nil
}
func NewService(deps Dependencies) Service {
	if deps.Expenses == nil {
		deps.Expenses = NoopExpenseCounter{}
	}
	if deps.Codes == nil {
		deps.Codes = CryptoCodeGenerator{}
	}
	return &service{deps: deps}
}
func (s *service) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (*domaintrip.Trip, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseCurrency = strings.ToUpper(strings.TrimSpace(in.BaseCurrency))
	if !currency(in.BaseCurrency) {
		return nil, apperror.New("INVALID_CURRENCY")
	}
	country := trimmedOrNil(in.Country)
	if in.EndDate.Before(in.StartDate) {
		return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "end_date", Rule: "dateafter", Message: "end_date must be on or after start_date"}})
	}
	var code string
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := s.deps.Codes.Generate()
		if err != nil {
			return nil, apperror.Wrap(err, "INTERNAL_ERROR")
		}
		exists, err := s.deps.Repo.ExistsByCode(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if !exists {
			code = candidate
			break
		}
	}
	if code == "" {
		return nil, apperror.New("TRIP_CODE_COLLISION")
	}
	entity := &domaintrip.Trip{ID: uuid.New(), Code: code, Name: in.Name, BaseCurrency: in.BaseCurrency, Country: country, StartDate: date(in.StartDate), EndDate: date(in.EndDate), PlannerID: actor, Settings: in.Settings, Version: 1}
	part := &domainparticipant.Participant{ID: uuid.New(), TripID: entity.ID, UserID: actor, Role: domainparticipant.RolePlanner, JoinedAt: time.Now().UTC()}
	return s.deps.Tx.CreateTripWithPlanner(ctx, entity, part)
}
func (s *service) GetByCode(ctx context.Context, actor uuid.UUID, code string) (*domaintrip.Trip, error) {
	if authorized, ok := tripctx.ForActor(ctx, actor, code); ok {
		return &authorized.Trip, nil
	}
	entity, err := s.deps.Repo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if _, err = s.deps.Participants.GetByTripAndUser(ctx, entity.ID, actor); err != nil {
		return nil, apperror.New("NOT_TRIP_MEMBER")
	}
	return entity, nil
}
func (s *service) FindByCode(ctx context.Context, code string) (*domaintrip.Trip, error) {
	return s.deps.Repo.GetByCode(ctx, code)
}
func (s *service) ListMine(ctx context.Context, actor uuid.UUID, f ListFilter) ([]domaintrip.Trip, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 100 {
		f.PerPage = 20
	}
	return s.deps.Repo.ListByUserID(ctx, actor, f)
}
func (s *service) UpdateSettings(ctx context.Context, actor uuid.UUID, code string, in UpdateSettingsInput) (*domaintrip.Trip, error) {
	entity, err := s.plannerTrip(ctx, actor, code)
	if err != nil {
		return nil, err
	}
	if err = entity.AssertMutable(); err != nil {
		return nil, err
	}
	count, err := s.deps.Expenses.CountByTrip(ctx, entity.ID)
	if err != nil {
		return nil, err
	}
	if in.BaseCurrency != nil {
		next := strings.ToUpper(strings.TrimSpace(*in.BaseCurrency))
		if !currency(next) {
			return nil, apperror.New("INVALID_CURRENCY")
		}
		if count > 0 && next != entity.BaseCurrency {
			return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "base_currency", Rule: "immutable", Message: "base_currency cannot change after expenses exist"}})
		}
		entity.BaseCurrency = next
	}
	if !in.Settings.MultiCurrencyEnabled {
		currencies, err := s.deps.Expenses.CurrenciesByTrip(ctx, entity.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range currencies {
			if !strings.EqualFold(c, entity.BaseCurrency) {
				return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "multi_currency_enabled", Rule: "in_use", Message: "non-base currencies are in use: " + strings.Join(currencies, ", ")}})
			}
		}
	}
	if in.Name != nil {
		entity.Name = strings.TrimSpace(*in.Name)
	}
	entity.Settings = in.Settings
	entity.Version = in.Version
	return s.deps.Repo.Update(ctx, entity)
}

func (s *service) plannerTrip(ctx context.Context, actor uuid.UUID, code string) (*domaintrip.Trip, error) {
	if authorized, ok := tripctx.ForActor(ctx, actor, code); ok {
		if authorized.Participant.Role != domainparticipant.RolePlanner {
			return nil, apperror.New("PLANNER_ONLY")
		}
		return &authorized.Trip, nil
	}
	entity, err := s.deps.Repo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	member, err := s.deps.Participants.GetByTripAndUser(ctx, entity.ID, actor)
	if err != nil || member.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	return entity, nil
}
func date(v time.Time) time.Time {
	y, m, d := v.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// trimmedOrNil collapses an absent, empty, or whitespace-only optional string down to nil, so a
// trip created with country:"" (or country:"  ") stores the same "no country set" state as
// omitting the field entirely.
func trimmedOrNil(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
func currency(v string) bool {
	switch v {
	case "AUD", "CAD", "CHF", "CNY", "EUR", "GBP", "HKD", "IDR", "INR", "JPY", "KRW", "MYR", "NZD", "PHP", "SGD", "THB", "USD", "VND":
		return true
	}
	return false
}
