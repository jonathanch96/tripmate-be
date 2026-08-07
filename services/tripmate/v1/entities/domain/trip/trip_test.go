package trip

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
)

func TestCanEditTruthTable(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	tests := []struct {
		permission   EditPermission
		actor, owner uuid.UUID
		want         bool
	}{{EditEveryone, a, b, true}, {EditOwnOnly, a, a, true}, {EditOwnOnly, a, b, false}}
	for _, tt := range tests {
		trip := Trip{Settings: Settings{EditPermission: tt.permission}}
		if got := trip.CanEdit(tt.actor, tt.owner); got != tt.want {
			t.Fatalf("%s got %v", tt.permission, got)
		}
	}
}
func TestTripBehaviors(t *testing.T) {
	now := time.Now().UTC()
	trip := Trip{BaseCurrency: "USD", EndDate: now.Add(-48 * time.Hour), Settings: Settings{ApprovalRequiredExpenses: true, ApprovalRequiredSettlements: true}}
	if !trip.HasEnded(now) || !trip.RequiresApprovalForExpense() || !trip.RequiresApprovalForSettlement() {
		t.Fatal("behavior mismatch")
	}
	trip.Settings.MultiCurrencyEnabled = false
	if trip.AllowsCurrency("EUR") || !trip.AllowsCurrency("usd") {
		t.Fatal("currency behavior mismatch")
	}
	trip.IsFinalized = true
	if err := trip.AssertMutable(); !apperror.Is(err, "TRIP_FINALIZED") {
		t.Fatalf("mutable=%v", err)
	}
}
