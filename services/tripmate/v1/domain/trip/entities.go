package trip

import (
	"context"
	"time"

	"github.com/google/uuid"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type Dependencies struct {
	Repo         Repository
	Participants ParticipantRepository
	Tx           Transactor
	Expenses     ExpenseCounter
	Codes        CodeGenerator
	FX           FXProvider
}
type service struct{ deps Dependencies }
type CreateInput struct {
	Name, BaseCurrency string
	Country            *string
	StartDate, EndDate time.Time
	Settings           domaintrip.Settings
}
type UpdateSettingsInput struct {
	Name         *string
	BaseCurrency *string
	Country      *string
	Settings     domaintrip.Settings
	Version      int
}
type NoopExpenseCounter struct{}

func (NoopExpenseCounter) CountByTrip(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (NoopExpenseCounter) CurrenciesByTrip(context.Context, uuid.UUID) ([]string, error) {
	return []string{}, nil
}

// NoopFXProvider stands in when no exchange rates are configured anywhere - every non-base-currency
// conversion simply reports the rate as missing, same as a trip with no rates saved yet.
type NoopFXProvider struct{}

func (NoopFXProvider) EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error) {
	return fxdomain.NewRateTable(nil), nil
}
