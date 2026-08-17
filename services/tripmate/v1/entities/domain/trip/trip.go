package trip

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/shopspring/decimal"
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
	// Country is a free-form, planner-entered label (e.g. "Japan") with no relation to
	// BaseCurrency - it exists purely to group trips on the cross-trip analytics page and is
	// never required.
	Country              *string
	StartDate, EndDate   time.Time
	PlannerID            uuid.UUID
	IsFinalized          bool
	FinalizedAt          *time.Time
	IsArchived           bool
	ArchivedAt           *time.Time
	Settings             Settings
	Version              int
	CreatedAt, UpdatedAt time.Time
	// Currencies is every currency actually used on the trip (base currency plus whatever its
	// expenses are recorded in). Only populated by the trip list query, for the "My trips" cards -
	// nil elsewhere.
	Currencies []string
	// TotalSpend is every approved expense converted into BaseCurrency. Only populated by the trip
	// list query, for the "My trips" cards - nil elsewhere.
	TotalSpend *decimal.Decimal
	// MemberCount is the trip's active participant count. Only populated by the trip list query -
	// zero elsewhere.
	MemberCount int
	// ExpenseTotals holds each currency's raw (unconverted) approved-expense sum, as loaded by the
	// trip list query. It exists only so ListMine can convert it into TotalSpend once trip-scoped
	// exchange rates are available, and is never exposed in an API response.
	ExpenseTotals map[string]decimal.Decimal
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
	if t.IsArchived {
		return apperror.New("TRIP_ARCHIVED")
	}
	return nil
}
