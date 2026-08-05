package trip

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
)

type EditPermission string

const (
	EditEveryone EditPermission = "everyone"
	EditOwnOnly  EditPermission = "own_only"
)

type Settings struct {
	EditPermission                                                                                        EditPermission
	ApprovalRequiredExpenses, ApprovalRequiredSettlements, MultiCurrencyEnabled, AllowSettlementBeforeEnd bool
}
type Trip struct {
	ID                       uuid.UUID
	Code, Name, BaseCurrency string
	StartDate, EndDate       time.Time
	PlannerID                uuid.UUID
	IsFinalized              bool
	FinalizedAt              *time.Time
	Settings                 Settings
	Version                  int
	CreatedAt, UpdatedAt     time.Time
}

func (t Trip) HasEnded(now time.Time) bool {
	return now.UTC().After(t.EndDate.UTC().Add(24*time.Hour - time.Nanosecond))
}
func (t Trip) CanEdit(actorID, ownerID uuid.UUID) bool {
	return t.Settings.EditPermission == EditEveryone || actorID == ownerID
}
func (t Trip) RequiresApprovalForExpense() bool    { return t.Settings.ApprovalRequiredExpenses }
func (t Trip) RequiresApprovalForSettlement() bool { return t.Settings.ApprovalRequiredSettlements }
func (t Trip) AllowsCurrency(code string) bool {
	return t.Settings.MultiCurrencyEnabled || strings.EqualFold(code, t.BaseCurrency)
}
func (t Trip) AssertMutable() error {
	if t.IsFinalized {
		return apperror.New("TRIP_FINALIZED")
	}
	return nil
}
