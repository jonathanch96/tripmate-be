package receipt

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/shopspring/decimal"
)

// Test-plan item 10: the lifecycle, stated exhaustively. Every pair of statuses is checked, so a
// transition silently added or dropped in the map fails here rather than in production.
func TestStatusTransitions(t *testing.T) {
	all := []Status{StatusUploaded, StatusExtracting, StatusExtracted, StatusFailed, StatusConverted}
	legal := map[Status]map[Status]bool{
		StatusUploaded:   {StatusExtracting: true},
		StatusExtracting: {StatusExtracted: true, StatusFailed: true},
		// Re-extraction is allowed (R-2 guards it separately), as is conversion.
		StatusExtracted: {StatusExtracting: true, StatusConverted: true},
		StatusFailed:    {StatusExtracting: true},
		// C-7: deleting the produced expense returns the receipt to extracted.
		StatusConverted: {StatusExtracted: true},
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s → %s = %v, want %v", from, to, got, want)
			}
			err := from.AssertTransition(to)
			if want && err != nil {
				t.Errorf("%s → %s returned %v", from, to, err)
			}
			if !want && !apperror.Is(err, "VALIDATION_FAILED") {
				t.Errorf("%s → %s error = %v, want VALIDATION_FAILED", from, to, err)
			}
		}
	}
}

func TestStatusNeverTransitionsToItself(t *testing.T) {
	for _, status := range []Status{StatusUploaded, StatusExtracting, StatusExtracted, StatusFailed, StatusConverted} {
		if status.CanTransitionTo(status) {
			t.Errorf("%s may transition to itself", status)
		}
	}
}

func TestConvertedReceiptIsReadOnly(t *testing.T) {
	if err := (Receipt{Status: StatusExtracted}).Editable(); err != nil {
		t.Fatalf("an extracted receipt must be editable: %v", err)
	}
	if err := (Receipt{Status: StatusConverted}).Editable(); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("Editable() on a converted receipt = %v", err)
	}
}

func TestOnlyUploaderOrPlannerMayEdit(t *testing.T) {
	uploader, other := uuid.New(), uuid.New()
	entity := Receipt{UploadedBy: uploader}
	if err := entity.EditableBy(uploader, false); err != nil {
		t.Fatalf("the uploader must be allowed: %v", err)
	}
	if err := entity.EditableBy(other, true); err != nil {
		t.Fatalf("a planner must be allowed: %v", err)
	}
	if err := entity.EditableBy(other, false); !apperror.Is(err, "FORBIDDEN") {
		t.Fatalf("EditableBy(bystander) = %v, want FORBIDDEN", err)
	}
}

func TestAssignmentAndTotalHelpers(t *testing.T) {
	tax, service, total := decimal.RequireFromString("12"), decimal.RequireFromString("8"), decimal.RequireFromString("120")
	entity := Receipt{Tax: &tax, ServiceCharge: &service, Total: &total, Items: []Item{
		{Name: "Pad thai", TotalPrice: decimal.RequireFromString("60"), Assignees: []uuid.UUID{uuid.New()}},
		{Name: "Beer", TotalPrice: decimal.RequireFromString("40")},
	}}
	if !entity.Extras().Equal(decimal.RequireFromString("20")) {
		t.Fatalf("Extras() = %s, want 20", entity.Extras())
	}
	if !entity.ItemsTotal().Equal(decimal.RequireFromString("100")) {
		t.Fatalf("ItemsTotal() = %s, want 100", entity.ItemsTotal())
	}
	if !entity.AmountDue().Equal(total) {
		t.Fatalf("AmountDue() = %s, want the parsed total %s", entity.AmountDue(), total)
	}
	if !entity.HasAssignments() {
		t.Fatal("HasAssignments() = false with one claimed line")
	}
	unassigned := entity.UnassignedItems()
	if len(unassigned) != 1 || unassigned[0].Name != "Beer" {
		t.Fatalf("UnassignedItems() = %+v, want just the beer", unassigned)
	}
}

// OCR-5: a receipt whose total the model never found still has an amount due.
func TestAmountDueFallsBackToItemsPlusExtras(t *testing.T) {
	tax := decimal.RequireFromString("10")
	entity := Receipt{Tax: &tax, Items: []Item{{TotalPrice: decimal.RequireFromString("90")}}}
	if !entity.AmountDue().Equal(decimal.RequireFromString("100")) {
		t.Fatalf("AmountDue() = %s, want 100", entity.AmountDue())
	}
	if entity.HasAssignments() {
		t.Fatal("HasAssignments() = true with no claimed lines")
	}
}
