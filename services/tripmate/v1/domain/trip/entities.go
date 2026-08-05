package trip

import (
	"context"
	"github.com/google/uuid"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"time"
)

type Dependencies struct {
	Repo         Repository
	Participants ParticipantRepository
	Tx           Transactor
	Expenses     ExpenseCounter
	Codes        CodeGenerator
}
type service struct{ deps Dependencies }
type CreateInput struct {
	Name, BaseCurrency string
	StartDate, EndDate time.Time
	Settings           domaintrip.Settings
}
type UpdateSettingsInput struct {
	Name         *string
	BaseCurrency *string
	Settings     domaintrip.Settings
	Version      int
}
type NoopExpenseCounter struct{}

func (NoopExpenseCounter) CountByTrip(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (NoopExpenseCounter) CurrenciesByTrip(context.Context, uuid.UUID) ([]string, error) {
	return []string{}, nil
}
