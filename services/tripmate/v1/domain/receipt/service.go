package receipt

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/storage"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainreceipt "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/receipt"
	"github.com/shopspring/decimal"
)

type Dependencies struct {
	Repo         Repository
	Participants ParticipantRepository
	Storage      ObjectStorage
	OCR          OCRProvider
	Expenses     ExpenseCreator
	UOW          UnitOfWork
	Clock        func() time.Time
}

type service struct{ deps Dependencies }

func NewService(deps Dependencies) Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &service{deps: deps}
}

// Upload is deliberately the cheap half of the story (rule R-1). It sniffs, strips, and stores the
// image, and nothing else — so a flaky model never costs the user their photograph.
func (s *service) Upload(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, in UploadInput) (*domainreceipt.Receipt, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	if err := storage.AssertSize(int64(len(in.Data))); err != nil {
		return nil, err
	}
	contentType, err := storage.DetectImageType(in.Data)
	if err != nil {
		return nil, err
	}
	// ST-5: GPS and the rest of the EXIF block never reach the bucket.
	clean := storage.StripMetadata(in.Data)

	now := s.deps.Clock().UTC()
	entity := &domainreceipt.Receipt{
		ID: uuid.New(), TripID: tc.Trip.ID, UploadedBy: actor.UserID,
		ContentType: contentType, SizeBytes: int64(len(clean)),
		Status: domainreceipt.StatusUploaded, CreatedAt: now, UpdatedAt: now,
	}
	entity.StorageKey = storage.Key(tc.Trip.ID, entity.ID, contentType)

	if err = s.deps.Storage.Put(ctx, entity.StorageKey, bytes.NewReader(clean), int64(len(clean)), contentType); err != nil {
		return nil, err
	}
	created, err := s.deps.Repo.Create(ctx, entity)
	if err != nil {
		// The row is the record; an orphaned object would linger until the lifecycle rule collects
		// it, so clean it up now rather than leave litter behind.
		_ = s.deps.Storage.Delete(ctx, entity.StorageKey)
		return nil, err
	}
	return created, nil
}

// Extract is the half that can fail (rule R-1). Whatever the model says is persisted — including a
// failure, with the raw response kept for whoever has to debug it.
func (s *service) Extract(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, receiptID uuid.UUID) (*domainreceipt.Receipt, error) {
	entity, err := s.editable(ctx, actor, tc, receiptID)
	if err != nil {
		return nil, err
	}
	// R-2: re-extraction throws the lines away, so it is refused once anyone has claimed one.
	if entity.HasAssignments() {
		return nil, apperror.Newf("VALIDATION_FAILED", "unassign the receipt's items before extracting it again")
	}
	if err = entity.Status.AssertTransition(domainreceipt.StatusExtracting); err != nil {
		return nil, err
	}

	image, err := s.deps.Storage.Get(ctx, entity.StorageKey)
	if err != nil {
		return nil, err
	}
	result, extractErr := s.deps.OCR.Extract(ctx, Image{Data: image, ContentType: entity.ContentType})
	provider, model := s.deps.OCR.Name(), s.deps.OCR.Model()
	entity.Provider, entity.ProviderModel = &provider, &model

	if extractErr != nil {
		// OCR-2: the receipt lands in `failed`, keeping enough to retry or to enter by hand.
		reason := failureReason(extractErr)
		entity.Status, entity.FailureReason = domainreceipt.StatusFailed, &reason
		if _, err = s.deps.Repo.Update(ctx, entity); err != nil {
			return nil, err
		}
		return nil, extractErr
	}

	applyExtraction(entity, result, tc.Trip.BaseCurrency)
	err = s.deps.UOW.Do(ctx, func(tx context.Context) error {
		if _, updateErr := s.deps.Repo.Update(tx, entity); updateErr != nil {
			return updateErr
		}
		return s.deps.Repo.ReplaceItems(tx, entity.ID, entity.Items)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Repo.GetByID(ctx, entity.ID)
}

// applyExtraction copies a model's reading onto the receipt. The trip's own currency is the
// fallback when the model could not name one, so downstream conversion always has something valid.
func applyExtraction(entity *domainreceipt.Receipt, result *ExtractionResult, baseCurrency string) {
	merchant := strings.TrimSpace(result.Merchant)
	if merchant != "" {
		entity.Merchant = &merchant
	}
	entity.ReceiptDate = result.Date
	currency := result.Currency
	if currency == "" {
		currency = strings.ToUpper(baseCurrency)
	}
	entity.Currency = &currency
	subtotal, tax, service, total := result.Subtotal, result.Tax, result.ServiceCharge, result.Total
	entity.Subtotal, entity.Tax, entity.ServiceCharge, entity.Total = &subtotal, &tax, &service, &total
	entity.RawResponse = result.Raw
	entity.Status, entity.FailureReason = domainreceipt.StatusExtracted, nil

	entity.Items = make([]domainreceipt.Item, len(result.Items))
	for index, item := range result.Items {
		entity.Items[index] = domainreceipt.Item{
			ID: uuid.New(), ReceiptID: entity.ID, Name: item.Name, Quantity: item.Quantity,
			UnitPrice: item.UnitPrice, TotalPrice: item.TotalPrice, SortOrder: index,
		}
	}
}

func (s *service) Get(ctx context.Context, tc tripctx.TripContext, receiptID uuid.UUID) (*domainreceipt.Receipt, error) {
	entity, err := s.owned(ctx, tc, receiptID)
	if err != nil {
		return nil, err
	}
	// ST-2: the link is minted per request and expires; it is never stored on the row.
	if link, linkErr := s.deps.Storage.PresignedGet(ctx, entity.StorageKey, storage.PresignTTL); linkErr == nil {
		entity.ImageURL = link
	}
	return entity, nil
}

func (s *service) ListByTrip(ctx context.Context, tc tripctx.TripContext, f Filter) ([]domainreceipt.Receipt, int64, error) {
	return s.deps.Repo.ListByTripID(ctx, tc.Trip.ID, f)
}

// AddItem covers rule R-4: the model misses handwritten lines, so a human can add them.
func (s *service) AddItem(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, receiptID uuid.UUID, in AddItemInput) (*domainreceipt.Item, error) {
	entity, err := s.editable(ctx, actor, tc, receiptID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 255 {
		return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "name", Rule: "required", Message: "an item needs a name"}})
	}
	if in.TotalPrice.IsNegative() || in.UnitPrice.IsNegative() {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	quantity := in.Quantity
	if !quantity.IsPositive() {
		quantity = decimal.NewFromInt(1)
	}
	total := in.TotalPrice
	if total.IsZero() {
		total = in.UnitPrice.Mul(quantity)
	}
	now := s.deps.Clock().UTC()
	return s.deps.Repo.CreateItem(ctx, &domainreceipt.Item{
		ID: uuid.New(), ReceiptID: entity.ID, Name: name, Quantity: quantity,
		UnitPrice: in.UnitPrice, TotalPrice: total, SortOrder: len(entity.Items),
		CreatedAt: now, UpdatedAt: now,
	})
}

// UpdateItem covers rule R-3: OCR is a starting point, and correcting it is expected.
func (s *service) UpdateItem(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, itemID uuid.UUID, in UpdateItemInput) (*domainreceipt.Item, error) {
	item, err := s.deps.Repo.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if _, err = s.editable(ctx, actor, tc, item.ReceiptID); err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 255 {
			return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "name", Rule: "required", Message: "an item needs a name"}})
		}
		item.Name = name
	}
	if in.Quantity != nil {
		if !in.Quantity.IsPositive() {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		item.Quantity = *in.Quantity
	}
	if in.UnitPrice != nil {
		if in.UnitPrice.IsNegative() {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		item.UnitPrice = *in.UnitPrice
	}
	if in.TotalPrice != nil {
		if in.TotalPrice.IsNegative() {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		item.TotalPrice = *in.TotalPrice
	} else if in.UnitPrice != nil || in.Quantity != nil {
		// Correcting a unit price without restating the total means the total follows from it.
		item.TotalPrice = item.UnitPrice.Mul(item.Quantity)
	}
	return s.deps.Repo.UpdateItem(ctx, item)
}

func (s *service) DeleteItem(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, itemID uuid.UUID) error {
	item, err := s.deps.Repo.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if _, err = s.editable(ctx, actor, tc, item.ReceiptID); err != nil {
		return err
	}
	return s.deps.Repo.DeleteItem(ctx, itemID)
}

// SetAssignments replaces the whole grid at once (rule A-4). Everyone named has to be an active
// participant (A-1), and a line may be shared by any number of them (A-2).
func (s *service) SetAssignments(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, receiptID uuid.UUID, in []ItemAssignmentInput) error {
	entity, err := s.owned(ctx, tc, receiptID)
	if err != nil {
		return err
	}
	if err = entity.Editable(); err != nil {
		return err
	}
	// A-5: the grid is only meaningful once there are lines to assign.
	if entity.Status != domainreceipt.StatusExtracted {
		return apperror.Newf("VALIDATION_FAILED", "items can only be assigned once the receipt has been extracted")
	}
	active, err := s.activeParticipants(ctx, tc.Trip.ID)
	if err != nil {
		return err
	}
	known := make(map[uuid.UUID]struct{}, len(entity.Items))
	for _, item := range entity.Items {
		known[item.ID] = struct{}{}
	}

	byItem := make(map[uuid.UUID][]uuid.UUID, len(in))
	for _, row := range in {
		if _, ok := known[row.ItemID]; !ok {
			return apperror.New("RECEIPT_NOT_FOUND")
		}
		seen := make(map[uuid.UUID]struct{}, len(row.UserIDs))
		users := make([]uuid.UUID, 0, len(row.UserIDs))
		for _, userID := range row.UserIDs {
			if _, ok := active[userID]; !ok {
				return apperror.New("PARTICIPANT_NOT_FOUND")
			}
			if _, duplicate := seen[userID]; duplicate {
				continue
			}
			seen[userID] = struct{}{}
			users = append(users, userID)
		}
		if len(users) > 0 {
			byItem[row.ItemID] = users
		}
	}
	return s.deps.Repo.ReplaceAssignments(ctx, receiptID, byItem)
}

// Shares is the server-side answer to "who owes what" (F-05). The frontend renders this and never
// recomputes it, so there is exactly one implementation of the arithmetic.
func (s *service) Shares(ctx context.Context, tc tripctx.TripContext, receiptID uuid.UUID) (*ShareBreakdown, error) {
	entity, err := s.owned(ctx, tc, receiptID)
	if err != nil {
		return nil, err
	}
	return s.shares(entity)
}

func (s *service) shares(entity *domainreceipt.Receipt) (*ShareBreakdown, error) {
	currency := strings.ToUpper(deref(entity.Currency))
	breakdown := &ShareBreakdown{
		Currency: currency,
		Tax:      derefAmount(entity.Tax), ServiceCharge: derefAmount(entity.ServiceCharge),
	}
	for _, item := range entity.UnassignedItems() {
		breakdown.UnassignedItems = append(breakdown.UnassignedItems, item.ID)
	}

	assigned := make([]expensedomain.ItemAssignment, 0, len(entity.Items))
	for _, item := range entity.Items {
		if len(item.Assignees) == 0 {
			continue
		}
		breakdown.ItemsTotal = breakdown.ItemsTotal.Add(item.TotalPrice)
		assigned = append(assigned, expensedomain.ItemAssignment{Amount: item.TotalPrice, UserIDs: item.Assignees})
	}
	extras := breakdown.Tax.Add(breakdown.ServiceCharge)
	breakdown.Total = breakdown.ItemsTotal.Add(extras)
	// A-6: the lines do not agree with the printed subtotal. Worth flagging, not worth blocking,
	// because what people actually owe comes from the assignments.
	subtotal := derefAmount(entity.Subtotal)
	if !subtotal.IsZero() && !money.IsZero(entity.ItemsTotal().Sub(subtotal)) {
		breakdown.Discrepancy = true
	}
	if len(assigned) == 0 {
		return breakdown, nil
	}

	// The same calculator the expense domain uses, so a receipt's preview and the expense it
	// eventually creates cannot disagree.
	splits, err := expensedomain.CalculateSplits(expensedomain.SplitInput{
		Currency: currency, SplitType: domainexpense.SplitItem, Items: assigned, Extras: extras,
	})
	if err != nil {
		return nil, err
	}

	// Re-derive each diner's item subtotal so the breakdown can show items and extras separately.
	subtotals := itemSubtotals(assigned, currency)
	for _, split := range splits {
		itemsPart := subtotals[split.UserID]
		allocated := split.Amount.Sub(itemsPart)
		share := UserShare{UserID: split.UserID, ItemsSubtotal: itemsPart, Total: split.Amount}
		// Present the allocation split between tax and service in the same proportion they were
		// charged, so the two columns add up to what the diner actually pays.
		share.Tax, share.ServiceCharge = divideExtras(allocated, breakdown.Tax, breakdown.ServiceCharge, currency)
		breakdown.PerUser = append(breakdown.PerUser, share)
	}
	return breakdown, nil
}

func itemSubtotals(items []expensedomain.ItemAssignment, currency string) map[uuid.UUID]decimal.Decimal {
	subtotals := make(map[uuid.UUID]decimal.Decimal)
	for _, item := range items {
		assignees := append([]uuid.UUID(nil), item.UserIDs...)
		sort.Slice(assignees, func(i, j int) bool { return assignees[i].String() < assignees[j].String() })
		shares := money.SplitEqual(item.Amount, len(assignees), currency)
		for index, userID := range assignees {
			subtotals[userID] = subtotals[userID].Add(shares[index])
		}
	}
	return subtotals
}

// divideExtras splits one diner's allocated extras back into a tax part and a service part.
func divideExtras(allocated, tax, service decimal.Decimal, currency string) (decimal.Decimal, decimal.Decimal) {
	if allocated.IsZero() {
		return decimal.Zero, decimal.Zero
	}
	parts := money.SplitProportional(allocated, []decimal.Decimal{tax, service}, currency)
	return parts[0], parts[1]
}

// ConvertToExpense turns a receipt into exactly one expense (F-06): one row, N payers, splits taken
// from the same arithmetic the share preview showed.
func (s *service) ConvertToExpense(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, receiptID uuid.UUID, in ConvertInput) (*domainexpense.Expense, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	entity, err := s.owned(ctx, tc, receiptID)
	if err != nil {
		return nil, err
	}
	// C-6: a receipt produces one expense, once.
	if entity.Status == domainreceipt.StatusConverted {
		return nil, apperror.Newf("VALIDATION_FAILED", "this receipt has already been converted into an expense")
	}
	if entity.Status != domainreceipt.StatusExtracted {
		return nil, apperror.Newf("VALIDATION_FAILED", "only an extracted receipt can be converted")
	}
	breakdown, err := s.shares(entity)
	if err != nil {
		return nil, err
	}
	// A-3: an unclaimed line would quietly land on the payer, which is the prototype's bug.
	if len(breakdown.UnassignedItems) > 0 {
		return nil, unassignedError(entity, breakdown.UnassignedItems)
	}
	if len(breakdown.PerUser) == 0 {
		return nil, apperror.Newf("VALIDATION_FAILED", "assign the receipt's items before converting it")
	}

	currency := breakdown.Currency
	// C-4: the receipt's own currency still has to be one the trip allows.
	if !money.IsSupportedCurrency(currency) || !tc.Trip.AllowsCurrency(currency) {
		return nil, apperror.New("INVALID_CURRENCY")
	}

	payers := make([]domainexpense.Payer, len(in.Payers))
	for index, payer := range in.Payers {
		payers[index] = domainexpense.Payer{UserID: payer.UserID, Amount: payer.Amount}
	}
	// C-2 is enforced inside expense.Create via ValidatePayers against this amount.
	amount := breakdown.Total

	items := make([]expensedomain.ItemAssignment, 0, len(entity.Items))
	for _, item := range entity.Items {
		items = append(items, expensedomain.ItemAssignment{Amount: item.TotalPrice, UserIDs: item.Assignees})
	}

	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = strings.TrimSpace(deref(entity.Merchant))
	}
	if description == "" {
		description = "Receipt"
	}
	when := s.deps.Clock().UTC()
	switch {
	case in.ExpenseDate != nil:
		when = *in.ExpenseDate
	case entity.ReceiptDate != nil:
		when = *entity.ReceiptDate
	}

	var created *domainexpense.Expense
	// C-1: the expense, the link, and the status change are one transaction or none of them.
	err = s.deps.UOW.Do(ctx, func(tx context.Context) error {
		expense, createErr := s.deps.Expenses.Create(tx, actor, tc, expensedomain.CreateInput{
			ExpenseDate: when, Description: description, Amount: amount, Currency: currency,
			SplitType: domainexpense.SplitItem, Payers: payers, Items: items,
			Extras: breakdown.Tax.Add(breakdown.ServiceCharge), Source: domainexpense.SourceReceipt,
		})
		if createErr != nil {
			return createErr
		}
		// C-3: guaranteed by the exact-sum allocation, asserted anyway — this is money.
		total := decimal.Zero
		for _, split := range expense.Splits {
			total = total.Add(split.Amount)
		}
		if !total.Equal(amount) {
			return apperror.Newf("SPLIT_SUM_MISMATCH", "splits sum to %s, expected %s", total, amount)
		}
		entity.ExpenseID, entity.Status = &expense.ID, domainreceipt.StatusConverted
		if _, updateErr := s.deps.Repo.Update(tx, entity); updateErr != nil {
			return updateErr
		}
		created = expense
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Delete removes the receipt and its image but leaves any expense it produced alone (rule R-7).
func (s *service) Delete(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, receiptID uuid.UUID) error {
	entity, err := s.owned(ctx, tc, receiptID)
	if err != nil {
		return err
	}
	if err = entity.EditableBy(actor.UserID, tc.Participant.Role == domainparticipant.RolePlanner); err != nil {
		return err
	}
	if err = s.deps.Repo.SoftDelete(ctx, receiptID); err != nil {
		return err
	}
	// The row is gone either way; a storage hiccup should not fail the request.
	_ = s.deps.Storage.Delete(ctx, entity.StorageKey)
	return nil
}

// owned resolves a receipt and refuses one belonging to another trip, so a guessed id leaks nothing.
func (s *service) owned(ctx context.Context, tc tripctx.TripContext, receiptID uuid.UUID) (*domainreceipt.Receipt, error) {
	entity, err := s.deps.Repo.GetByID(ctx, receiptID)
	if err != nil {
		return nil, err
	}
	if entity.TripID != tc.Trip.ID {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return entity, nil
}

// editable is the common gate for anything that changes a receipt: it must belong to this trip, the
// trip must be open, the receipt must not be converted, and the actor must be its owner or a planner.
func (s *service) editable(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, receiptID uuid.UUID) (*domainreceipt.Receipt, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	entity, err := s.owned(ctx, tc, receiptID)
	if err != nil {
		return nil, err
	}
	if err = entity.Editable(); err != nil {
		return nil, err
	}
	if err = entity.EditableBy(actor.UserID, tc.Participant.Role == domainparticipant.RolePlanner); err != nil {
		return nil, err
	}
	return entity, nil
}

func (s *service) activeParticipants(ctx context.Context, tripID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	rows, err := s.deps.Participants.ListByTripID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	active := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		active[row.UserID] = struct{}{}
	}
	return active, nil
}

// unassignedError names the offending lines, because "some item is unassigned" is not actionable
// when a bill has thirty of them.
func unassignedError(entity *domainreceipt.Receipt, ids []uuid.UUID) error {
	names := make([]string, 0, len(ids))
	wanted := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	for _, item := range entity.Items {
		if _, ok := wanted[item.ID]; ok {
			names = append(names, item.Name)
		}
	}
	return apperror.Newf("VALIDATION_FAILED", "assign these items before converting: %s", strings.Join(names, ", "))
}

// failureReason records why extraction failed in a form a human can act on, without leaking
// anything the provider said about the request itself.
func failureReason(err error) string {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr.Code + ": " + appErr.Message
	}
	return "INTERNAL_ERROR"
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefAmount(value *decimal.Decimal) decimal.Decimal {
	if value == nil {
		return decimal.Zero
	}
	return *value
}
