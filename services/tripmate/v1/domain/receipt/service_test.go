package receipt

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/shopspring/decimal"
)

func amount(value string) decimal.Decimal { return decimal.RequireFromString(value) }

var (
	ana  = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	ben  = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	cara = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
)

type memoryRepo struct {
	receipt     *domainreceipt.Receipt
	replaced    []domainreceipt.Item
	assignments map[uuid.UUID][]uuid.UUID
	cleared     []uuid.UUID
}

func (r *memoryRepo) Create(_ context.Context, e *domainreceipt.Receipt) (*domainreceipt.Receipt, error) {
	copied := *e
	r.receipt = &copied
	return &copied, nil
}
func (r *memoryRepo) GetByID(context.Context, uuid.UUID) (*domainreceipt.Receipt, error) {
	if r.receipt == nil {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	copied := *r.receipt
	return &copied, nil
}
func (r *memoryRepo) Update(_ context.Context, e *domainreceipt.Receipt) (*domainreceipt.Receipt, error) {
	copied := *e
	r.receipt = &copied
	return &copied, nil
}
func (r *memoryRepo) ListByTripID(context.Context, uuid.UUID, Filter) ([]domainreceipt.Receipt, int64, error) {
	if r.receipt == nil {
		return nil, 0, nil
	}
	return []domainreceipt.Receipt{*r.receipt}, 1, nil
}
func (r *memoryRepo) SoftDelete(context.Context, uuid.UUID) error { r.receipt = nil; return nil }
func (r *memoryRepo) ReplaceItems(_ context.Context, _ uuid.UUID, items []domainreceipt.Item) error {
	r.replaced = items
	r.receipt.Items = items
	return nil
}
func (r *memoryRepo) CreateItem(_ context.Context, e *domainreceipt.Item) (*domainreceipt.Item, error) {
	r.receipt.Items = append(r.receipt.Items, *e)
	return e, nil
}
func (r *memoryRepo) UpdateItem(_ context.Context, e *domainreceipt.Item) (*domainreceipt.Item, error) {
	for index := range r.receipt.Items {
		if r.receipt.Items[index].ID == e.ID {
			r.receipt.Items[index] = *e
		}
	}
	return e, nil
}
func (r *memoryRepo) GetItem(_ context.Context, id uuid.UUID) (*domainreceipt.Item, error) {
	for _, item := range r.receipt.Items {
		if item.ID == id {
			copied := item
			return &copied, nil
		}
	}
	return nil, apperror.New("RECEIPT_NOT_FOUND")
}
func (r *memoryRepo) DeleteItem(_ context.Context, id uuid.UUID) error {
	kept := make([]domainreceipt.Item, 0, len(r.receipt.Items))
	for _, item := range r.receipt.Items {
		if item.ID != id {
			kept = append(kept, item)
		}
	}
	r.receipt.Items = kept
	return nil
}
func (r *memoryRepo) ReplaceAssignments(_ context.Context, _ uuid.UUID, byItem map[uuid.UUID][]uuid.UUID) error {
	r.assignments = byItem
	for index := range r.receipt.Items {
		r.receipt.Items[index].Assignees = byItem[r.receipt.Items[index].ID]
	}
	return nil
}
func (r *memoryRepo) ClearExpenseLink(_ context.Context, expenseID uuid.UUID) error {
	r.cleared = append(r.cleared, expenseID)
	return nil
}

type participants []uuid.UUID

func (p participants) ListByTripID(context.Context, uuid.UUID) ([]domainparticipant.Participant, error) {
	rows := make([]domainparticipant.Participant, len(p))
	for index, id := range p {
		rows[index] = domainparticipant.Participant{UserID: id}
	}
	return rows, nil
}

type memoryStorage struct {
	objects map[string][]byte
	deleted []string
}

func newStorage() *memoryStorage { return &memoryStorage{objects: map[string][]byte{}} }
func (s *memoryStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.objects[key] = data
	return nil
}
func (s *memoryStorage) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return data, nil
}
func (s *memoryStorage) PresignedGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.invalid/" + key, nil
}
func (s *memoryStorage) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return nil
}

type stubOCR struct {
	result *ExtractionResult
	err    error
	calls  int
}

func (o *stubOCR) Name() string  { return "stub" }
func (o *stubOCR) Model() string { return "fixture" }
func (o *stubOCR) Extract(context.Context, Image) (*ExtractionResult, error) {
	o.calls++
	return o.result, o.err
}

type stubExpenses struct {
	created *expensedomain.CreateInput
	err     error
}

func (e *stubExpenses) Create(_ context.Context, _ identity.Identity, _ tripctx.TripContext, in expensedomain.CreateInput) (*domainexpense.Expense, error) {
	if e.err != nil {
		return nil, e.err
	}
	e.created = &in
	splits, err := expensedomain.CalculateSplits(expensedomain.SplitInput{
		Amount: in.Amount, Currency: in.Currency, SplitType: in.SplitType, Items: in.Items, Extras: in.Extras,
	})
	if err != nil {
		return nil, err
	}
	return &domainexpense.Expense{ID: uuid.New(), Amount: in.Amount, Currency: in.Currency,
		SplitType: in.SplitType, Source: in.Source, Payers: in.Payers, Splits: splits}, nil
}

type uow struct{}

func (uow) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fixture struct {
	service  Service
	repo     *memoryRepo
	storage  *memoryStorage
	ocr      *stubOCR
	expenses *stubExpenses
	actor    identity.Identity
	tc       tripctx.TripContext
}

// extractedReceipt is a bill with three lines: one shared starter and one main each.
func extractedReceipt(tripID uuid.UUID) *domainreceipt.Receipt {
	subtotal, tax, service, total := amount("1000"), amount("100"), amount("50"), amount("1150")
	currency := "PHP"
	merchant := "Test Diner"
	return &domainreceipt.Receipt{
		ID: uuid.New(), TripID: tripID, UploadedBy: ana, StorageKey: "trips/x/receipts/y.jpg",
		ContentType: "image/jpeg", Merchant: &merchant, Currency: &currency,
		Subtotal: &subtotal, Tax: &tax, ServiceCharge: &service, Total: &total,
		Status: domainreceipt.StatusExtracted,
		Items: []domainreceipt.Item{
			{ID: uuid.New(), Name: "Starter", TotalPrice: amount("200"), Quantity: decimal.NewFromInt(1)},
			{ID: uuid.New(), Name: "Ana's main", TotalPrice: amount("500"), Quantity: decimal.NewFromInt(1)},
			{ID: uuid.New(), Name: "Ben's main", TotalPrice: amount("300"), Quantity: decimal.NewFromInt(1)},
		},
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	tripID := uuid.New()
	repo := &memoryRepo{receipt: extractedReceipt(tripID)}
	store := newStorage()
	store.objects[repo.receipt.StorageKey] = []byte{0xFF, 0xD8, 0xFF, 0xE0}
	ocr := &stubOCR{}
	expenses := &stubExpenses{}
	svc := NewService(Dependencies{
		Repo: repo, Participants: participants{ana, ben, cara}, Storage: store,
		OCR: ocr, Expenses: expenses, UOW: uow{},
		Clock: func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) },
	})
	tc := tripctx.TripContext{
		Trip:        domaintrip.Trip{ID: tripID, BaseCurrency: "PHP", Settings: domaintrip.Settings{MultiCurrencyEnabled: true}},
		Participant: domainparticipant.Participant{UserID: ana, Role: domainparticipant.RolePlanner},
	}
	return &fixture{service: svc, repo: repo, storage: store, ocr: ocr, expenses: expenses,
		actor: identity.Identity{UserID: ana}, tc: tc}
}

func (f *fixture) assignAll(t *testing.T) {
	t.Helper()
	items := f.repo.receipt.Items
	err := f.service.SetAssignments(context.Background(), f.actor, f.tc, f.repo.receipt.ID, []ItemAssignmentInput{
		{ItemID: items[0].ID, UserIDs: []uuid.UUID{ana, ben}}, // shared starter
		{ItemID: items[1].ID, UserIDs: []uuid.UUID{ana}},
		{ItemID: items[2].ID, UserIDs: []uuid.UUID{ben}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// F-05: the whole point of the sprint's arithmetic — item shares plus a proportional slice of the
// extras, adding up exactly.
func TestSharesAllocatesItemsAndExtrasExactly(t *testing.T) {
	f := newFixture(t)
	f.assignAll(t)

	breakdown, err := f.service.Shares(context.Background(), f.tc, f.repo.receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !breakdown.ItemsTotal.Equal(amount("1000")) || !breakdown.Total.Equal(amount("1150")) {
		t.Fatalf("items/total = %s/%s, want 1000/1150", breakdown.ItemsTotal, breakdown.Total)
	}
	if len(breakdown.PerUser) != 2 {
		t.Fatalf("per-user rows = %d, want 2", len(breakdown.PerUser))
	}
	// Ana: 100 of the starter + her 500 main = 600. Ben: 100 + 300 = 400.
	byUser := map[uuid.UUID]UserShare{}
	for _, share := range breakdown.PerUser {
		byUser[share.UserID] = share
	}
	if !byUser[ana].ItemsSubtotal.Equal(amount("600")) || !byUser[ben].ItemsSubtotal.Equal(amount("400")) {
		t.Fatalf("item subtotals = %s/%s, want 600/400", byUser[ana].ItemsSubtotal, byUser[ben].ItemsSubtotal)
	}
	// Extras of 150 split 60/40 by what each ate: 90 and 60.
	if !byUser[ana].Total.Equal(amount("690")) || !byUser[ben].Total.Equal(amount("460")) {
		t.Fatalf("totals = %s/%s, want 690/460", byUser[ana].Total, byUser[ben].Total)
	}
	// The tax and service columns must themselves add up to the allocated extras.
	for userID, share := range byUser {
		if !share.Tax.Add(share.ServiceCharge).Add(share.ItemsSubtotal).Equal(share.Total) {
			t.Fatalf("user %s columns do not reconcile: %+v", userID, share)
		}
	}
	sum := decimal.Zero
	for _, share := range breakdown.PerUser {
		sum = sum.Add(share.Total)
	}
	if !sum.Equal(breakdown.Total) {
		t.Fatalf("per-user totals sum to %s, want %s", sum, breakdown.Total)
	}
}

// Test-plan item 5: someone assigned only a free item pays no share of the tax.
func TestSharesGiveNoExtrasToADinerWhoAteNothing(t *testing.T) {
	f := newFixture(t)
	items := f.repo.receipt.Items
	f.repo.receipt.Items[2].TotalPrice = decimal.Zero
	subtotal, total := amount("700"), amount("850")
	f.repo.receipt.Subtotal, f.repo.receipt.Total = &subtotal, &total

	if err := f.service.SetAssignments(context.Background(), f.actor, f.tc, f.repo.receipt.ID, []ItemAssignmentInput{
		{ItemID: items[0].ID, UserIDs: []uuid.UUID{ana}},
		{ItemID: items[1].ID, UserIDs: []uuid.UUID{ana}},
		{ItemID: items[2].ID, UserIDs: []uuid.UUID{ben}}, // the free item
	}); err != nil {
		t.Fatal(err)
	}
	breakdown, err := f.service.Shares(context.Background(), f.tc, f.repo.receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	byUser := map[uuid.UUID]UserShare{}
	for _, share := range breakdown.PerUser {
		byUser[share.UserID] = share
	}
	if !byUser[ben].Total.IsZero() {
		t.Fatalf("ben owes %s, want nothing — he was assigned only a zero-priced line", byUser[ben].Total)
	}
	if !byUser[ana].Total.Equal(amount("850")) {
		t.Fatalf("ana owes %s, want the whole 850", byUser[ana].Total)
	}
}

// Test-plan item 6 / rule A-3: the prototype silently dropped unassigned lines onto the payer.
func TestConversionIsBlockedByAnUnassignedItemAndNamesIt(t *testing.T) {
	f := newFixture(t)
	items := f.repo.receipt.Items
	if err := f.service.SetAssignments(context.Background(), f.actor, f.tc, f.repo.receipt.ID, []ItemAssignmentInput{
		{ItemID: items[0].ID, UserIDs: []uuid.UUID{ana, ben}},
		{ItemID: items[1].ID, UserIDs: []uuid.UUID{ana}},
		// items[2] deliberately left unclaimed.
	}); err != nil {
		t.Fatal(err)
	}

	breakdown, err := f.service.Shares(context.Background(), f.tc, f.repo.receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(breakdown.UnassignedItems) != 1 || breakdown.UnassignedItems[0] != items[2].ID {
		t.Fatalf("unassigned = %v, want just the third line", breakdown.UnassignedItems)
	}

	_, err = f.service.ConvertToExpense(context.Background(), f.actor, f.tc, f.repo.receipt.ID, ConvertInput{
		Payers: []PayerInput{{UserID: ana, Amount: amount("1150")}},
	})
	if !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("error = %v, want VALIDATION_FAILED", err)
	}
	if message := err.(*apperror.Error).Message; !strings.Contains(message, "Ben's main") {
		t.Fatalf("message = %q, want it to name the unassigned line", message)
	}
}

// F-06: one receipt becomes exactly one expense, with N payers and item splits.
func TestConvertProducesOneItemSplitExpenseLinkedBackToTheReceipt(t *testing.T) {
	f := newFixture(t)
	f.assignAll(t)

	expense, err := f.service.ConvertToExpense(context.Background(), f.actor, f.tc, f.repo.receipt.ID, ConvertInput{
		Payers: []PayerInput{{UserID: ana, Amount: amount("800")}, {UserID: ben, Amount: amount("350")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if expense.SplitType != domainexpense.SplitItem || expense.Source != domainexpense.SourceReceipt {
		t.Fatalf("split type/source = %s/%s", expense.SplitType, expense.Source)
	}
	if len(expense.Payers) != 2 {
		t.Fatalf("payers = %d, want 2 — one expense with N payers, not N expenses", len(expense.Payers))
	}
	// C-3: the splits reconstruct the bill.
	sum := decimal.Zero
	for _, split := range expense.Splits {
		sum = sum.Add(split.Amount)
	}
	if !sum.Equal(amount("1150")) {
		t.Fatalf("splits sum to %s, want 1150", sum)
	}
	// The description falls back to the merchant when none was given.
	if f.expenses.created.Description != "Test Diner" {
		t.Fatalf("description = %q, want the merchant name", f.expenses.created.Description)
	}
	// C-1: the receipt is linked and marked converted.
	if f.repo.receipt.Status != domainreceipt.StatusConverted || f.repo.receipt.ExpenseID == nil {
		t.Fatalf("receipt = %s, expense link = %v", f.repo.receipt.Status, f.repo.receipt.ExpenseID)
	}

	// C-6: it does not happen twice.
	_, err = f.service.ConvertToExpense(context.Background(), f.actor, f.tc, f.repo.receipt.ID, ConvertInput{
		Payers: []PayerInput{{UserID: ana, Amount: amount("1150")}},
	})
	if !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("second conversion = %v, want VALIDATION_FAILED", err)
	}
}

// R-2: re-extraction discards the lines, so it is refused once anyone has claimed one.
func TestReExtractionIsRefusedWhileItemsAreAssigned(t *testing.T) {
	f := newFixture(t)
	f.assignAll(t)
	_, err := f.service.Extract(context.Background(), f.actor, f.tc, f.repo.receipt.ID)
	if !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("error = %v, want VALIDATION_FAILED", err)
	}
	if f.ocr.calls != 0 {
		t.Fatal("the model was called despite the refusal — that is wasted quota")
	}
	if message := err.(*apperror.Error).Message; !strings.Contains(message, "unassign") {
		t.Fatalf("message = %q, want it to say what to do", message)
	}
}

func TestMalformedExtractionPersistsRawResponse(t *testing.T) {
	f := newFixture(t)
	f.repo.receipt.Status = domainreceipt.StatusUploaded
	f.ocr.result = &ExtractionResult{Raw: []byte("not-json-from-provider")}
	f.ocr.err = apperror.New("OCR_UNPARSEABLE")
	_, err := f.service.Extract(context.Background(), f.actor, f.tc, f.repo.receipt.ID)
	if !apperror.Is(err, "OCR_UNPARSEABLE") {
		t.Fatalf("error = %v, want OCR_UNPARSEABLE", err)
	}
	if f.repo.receipt.Status != domainreceipt.StatusFailed || string(f.repo.receipt.RawResponse) != "not-json-from-provider" {
		t.Fatalf("failed receipt lost provider evidence: status=%s raw=%q", f.repo.receipt.Status, f.repo.receipt.RawResponse)
	}
}

// A-1: someone who is not on the trip cannot be put on the hook for a line.
func TestAssignmentsRejectAStranger(t *testing.T) {
	f := newFixture(t)
	err := f.service.SetAssignments(context.Background(), f.actor, f.tc, f.repo.receipt.ID, []ItemAssignmentInput{
		{ItemID: f.repo.receipt.Items[0].ID, UserIDs: []uuid.UUID{uuid.New()}},
	})
	if !apperror.Is(err, "PARTICIPANT_NOT_FOUND") {
		t.Fatalf("error = %v, want PARTICIPANT_NOT_FOUND", err)
	}
}

// R-5/R-6: a converted receipt is frozen, and a bystander cannot edit someone else's.
func TestEditingGuards(t *testing.T) {
	t.Run("a converted receipt is read-only", func(t *testing.T) {
		f := newFixture(t)
		f.repo.receipt.Status = domainreceipt.StatusConverted
		_, err := f.service.AddItem(context.Background(), f.actor, f.tc, f.repo.receipt.ID, AddItemInput{Name: "Late addition", TotalPrice: amount("10")})
		if !apperror.Is(err, "VALIDATION_FAILED") {
			t.Fatalf("error = %v, want VALIDATION_FAILED", err)
		}
	})
	t.Run("a bystander cannot edit another member's receipt", func(t *testing.T) {
		f := newFixture(t)
		f.tc.Participant = domainparticipant.Participant{UserID: cara, Role: domainparticipant.RoleParticipant}
		_, err := f.service.AddItem(context.Background(), identity.Identity{UserID: cara}, f.tc, f.repo.receipt.ID, AddItemInput{Name: "Nope", TotalPrice: amount("10")})
		if !apperror.Is(err, "FORBIDDEN") {
			t.Fatalf("error = %v, want FORBIDDEN", err)
		}
	})
	t.Run("the planner may edit anyone's receipt", func(t *testing.T) {
		f := newFixture(t)
		f.tc.Participant = domainparticipant.Participant{UserID: cara, Role: domainparticipant.RolePlanner}
		if _, err := f.service.AddItem(context.Background(), identity.Identity{UserID: cara}, f.tc, f.repo.receipt.ID, AddItemInput{Name: "Extra beer", TotalPrice: amount("95")}); err != nil {
			t.Fatalf("the planner was refused: %v", err)
		}
	})
}

// R-7: deleting a receipt must not take the expense it produced with it.
func TestDeletingAReceiptLeavesItsExpenseAlone(t *testing.T) {
	f := newFixture(t)
	f.assignAll(t)
	expense, err := f.service.ConvertToExpense(context.Background(), f.actor, f.tc, f.repo.receipt.ID, ConvertInput{
		Payers: []PayerInput{{UserID: ana, Amount: amount("1150")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	key := f.repo.receipt.StorageKey
	if err = f.service.Delete(context.Background(), f.actor, f.tc, f.repo.receipt.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.storage.deleted) != 1 || f.storage.deleted[0] != key {
		t.Fatalf("the image was not cleaned up: %v", f.storage.deleted)
	}
	if expense.ID == uuid.Nil {
		t.Fatal("the expense should still exist")
	}
}

// A-5: the grid only makes sense once there is something to assign.
func TestAssignmentsAreRefusedBeforeExtraction(t *testing.T) {
	f := newFixture(t)
	f.repo.receipt.Status = domainreceipt.StatusUploaded
	err := f.service.SetAssignments(context.Background(), f.actor, f.tc, f.repo.receipt.ID, []ItemAssignmentInput{
		{ItemID: f.repo.receipt.Items[0].ID, UserIDs: []uuid.UUID{ana}},
	})
	if !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("error = %v, want VALIDATION_FAILED", err)
	}
}
