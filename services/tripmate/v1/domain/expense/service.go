package expense

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	"github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event"
	expenseevent "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/event/expense"
)

type service struct{ deps Dependencies }

func NewService(deps Dependencies) Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &service{deps: deps}
}

func (s *service) Create(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, input CreateInput) (*domainexpense.Expense, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	if err := validateDate(tc, input.ExpenseDate); err != nil {
		return nil, err
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if !money.IsSupportedCurrency(input.Currency) || !tc.Trip.AllowsCurrency(input.Currency) {
		return nil, apperror.New("INVALID_CURRENCY")
	}
	if !input.Amount.IsPositive() {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	if strings.TrimSpace(input.Description) == "" || len(strings.TrimSpace(input.Description)) > 255 {
		return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "description", Rule: "required", Message: "description is required"}})
	}
	if err := ValidatePayers(input.Amount, input.Currency, input.Payers); err != nil {
		return nil, err
	}
	if input.CategoryID != nil {
		if err := s.validateCategory(ctx, tc.Trip.ID, *input.CategoryID); err != nil {
			return nil, err
		}
	}
	splits, err := CalculateSplits(SplitInput{Amount: input.Amount, Currency: input.Currency, SplitType: input.SplitType, Participants: input.Participants, Manual: input.Manual, Weights: input.Weights, Items: input.Items, Extras: input.Extras})
	if err != nil {
		return nil, err
	}
	active, err := s.activeParticipants(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	if err = ValidateExpenseParticipants(active, input.Payers, splits); err != nil {
		return nil, err
	}
	status := domainexpense.StatusApproved
	if tc.Trip.Settings.ApprovalRequiredExpenses {
		status = domainexpense.StatusPending
	}
	now := s.deps.Clock().UTC()
	entity := &domainexpense.Expense{ID: uuid.New(), TripID: tc.Trip.ID, CategoryID: input.CategoryID, ExpenseDate: date(input.ExpenseDate),
		Description: strings.TrimSpace(input.Description), Amount: input.Amount, Currency: input.Currency,
		SplitType: input.SplitType, Status: status, Source: source(input.Source), Note: input.Note,
		CreatedByUserID: actor.UserID, Version: 1, Payers: input.Payers, Splits: splits, CreatedAt: now, UpdatedAt: now}
	err = s.deps.UOW.Do(ctx, func(txctx context.Context) error {
		if _, createErr := s.deps.Expenses.Create(txctx, entity); createErr != nil {
			return createErr
		}
		if createErr := s.deps.Payers.BulkCreate(txctx, entity.ID, entity.Payers); createErr != nil {
			return createErr
		}
		if createErr := s.deps.Splits.BulkCreate(txctx, entity.ID, entity.Splits); createErr != nil {
			return createErr
		}
		return s.emit(txctx, "expense.created", actor.UserID, tc, entity)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Expenses.GetByID(ctx, entity.ID)
}

func (s *service) Update(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID, input UpdateInput) (*domainexpense.Expense, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	entity, err := s.expense(ctx, tc, id)
	if err != nil {
		return nil, err
	}
	if !canEdit(actor.UserID, tc, entity.CreatedByUserID) {
		return nil, apperror.New("EDIT_OWN_ONLY")
	}
	if input.ExpenseDate != nil {
		if err = validateDate(tc, *input.ExpenseDate); err != nil {
			return nil, err
		}
		entity.ExpenseDate = date(*input.ExpenseDate)
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if value == "" || len(value) > 255 {
			return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "description", Rule: "required", Message: "description is required"}})
		}
		entity.Description = value
	}
	if input.Note != nil {
		entity.Note = input.Note
	}
	if input.Amount != nil {
		entity.Amount = *input.Amount
	}
	if input.Currency != nil {
		entity.Currency = strings.ToUpper(strings.TrimSpace(*input.Currency))
	}
	if input.SplitType != nil {
		entity.SplitType = *input.SplitType
	}
	if input.CategoryID != nil {
		if *input.CategoryID == uuid.Nil {
			entity.CategoryID = nil
		} else {
			if err = s.validateCategory(ctx, tc.Trip.ID, *input.CategoryID); err != nil {
				return nil, err
			}
			entity.CategoryID = input.CategoryID
		}
	}
	if !money.IsSupportedCurrency(entity.Currency) || !tc.Trip.AllowsCurrency(entity.Currency) {
		return nil, apperror.New("INVALID_CURRENCY")
	}
	rowsChanged := input.Amount != nil || input.Currency != nil || input.SplitType != nil || input.Payers != nil || input.Participants != nil || input.Manual != nil || input.Weights != nil
	if rowsChanged {
		if err = ValidatePayers(entity.Amount, entity.Currency, input.Payers); err != nil {
			return nil, err
		}
		entity.Payers = input.Payers
		entity.Splits, err = CalculateSplits(SplitInput{Amount: entity.Amount, Currency: entity.Currency, SplitType: entity.SplitType, Participants: input.Participants, Manual: input.Manual, Weights: input.Weights})
		if err != nil {
			return nil, err
		}
		active, activeErr := s.activeParticipants(ctx, tc.Trip.ID)
		if activeErr != nil {
			return nil, activeErr
		}
		if err = ValidateExpenseParticipants(active, entity.Payers, entity.Splits); err != nil {
			return nil, err
		}
	}
	if tc.Trip.Settings.ApprovalRequiredExpenses && (entity.Status == domainexpense.StatusApproved || entity.Status == domainexpense.StatusRejected) {
		entity.Status = domainexpense.StatusPending
		entity.ApprovedAt, entity.ApprovedByUserID, entity.RejectedReason = nil, nil, nil
	}
	entity.Version = input.Version
	err = s.deps.UOW.Do(ctx, func(txctx context.Context) error {
		if _, updateErr := s.deps.Expenses.Update(txctx, entity); updateErr != nil {
			return updateErr
		}
		if rowsChanged {
			if updateErr := s.deps.Payers.DeleteByExpenseID(txctx, entity.ID); updateErr != nil {
				return updateErr
			}
			if updateErr := s.deps.Splits.DeleteByExpenseID(txctx, entity.ID); updateErr != nil {
				return updateErr
			}
			if updateErr := s.deps.Payers.BulkCreate(txctx, entity.ID, entity.Payers); updateErr != nil {
				return updateErr
			}
			if updateErr := s.deps.Splits.BulkCreate(txctx, entity.ID, entity.Splits); updateErr != nil {
				return updateErr
			}
		}
		return s.emit(txctx, "expense.updated", actor.UserID, tc, entity)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Expenses.GetByID(ctx, entity.ID)
}

func (s *service) Delete(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID) error {
	if err := tc.Trip.AssertMutable(); err != nil {
		return err
	}
	entity, err := s.expense(ctx, tc, id)
	if err != nil {
		return err
	}
	if !canEdit(actor.UserID, tc, entity.CreatedByUserID) {
		return apperror.New("EDIT_OWN_ONLY")
	}
	return s.deps.UOW.Do(ctx, func(txctx context.Context) error {
		if s.deps.Receipts != nil {
			if unlinkErr := s.deps.Receipts.ClearExpenseLink(txctx, id); unlinkErr != nil {
				return unlinkErr
			}
		}
		if deleteErr := s.deps.Expenses.SoftDelete(txctx, id); deleteErr != nil {
			return deleteErr
		}
		return s.emit(txctx, "expense.deleted", actor.UserID, tc, entity)
	})
}

func (s *service) Approve(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID) (*domainexpense.Expense, error) {
	return s.transition(ctx, actor, tc, id, domainexpense.StatusApproved, "")
}

func (s *service) Reject(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID, reason string) (*domainexpense.Expense, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	return s.transition(ctx, actor, tc, id, domainexpense.StatusRejected, strings.TrimSpace(reason))
}

func (s *service) transition(ctx context.Context, actor identity.Identity, tc tripctx.TripContext, id uuid.UUID, status domainexpense.Status, reason string) (*domainexpense.Expense, error) {
	if err := tc.Trip.AssertMutable(); err != nil {
		return nil, err
	}
	if tc.Participant.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	entity, err := s.expense(ctx, tc, id)
	if err != nil {
		return nil, err
	}
	if entity.Status != domainexpense.StatusPending {
		return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "status", Rule: "pending", Message: "expense is not pending"}})
	}
	now := s.deps.Clock().UTC()
	entity.Status = status
	if status == domainexpense.StatusApproved {
		entity.ApprovedByUserID = &actor.UserID
		entity.ApprovedAt = &now
		entity.RejectedReason = nil
	} else {
		entity.RejectedReason = &reason
		entity.ApprovedByUserID, entity.ApprovedAt = nil, nil
	}
	eventType := "expense.approved"
	if status == domainexpense.StatusRejected {
		eventType = "expense.rejected"
	}
	err = s.deps.UOW.Do(ctx, func(txctx context.Context) error {
		if _, updateErr := s.deps.Expenses.Update(txctx, entity); updateErr != nil {
			return updateErr
		}
		return s.emit(txctx, eventType, actor.UserID, tc, entity)
	})
	if err != nil {
		return nil, err
	}
	return s.deps.Expenses.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, tc tripctx.TripContext, filter Filter) ([]domainexpense.Expense, int64, Totals, error) {
	return s.deps.Expenses.ListByTripID(ctx, tc.Trip.ID, filter)
}

func (s *service) Get(ctx context.Context, tc tripctx.TripContext, id uuid.UUID) (*domainexpense.Expense, error) {
	return s.expense(ctx, tc, id)
}

func (s *service) expense(ctx context.Context, tc tripctx.TripContext, id uuid.UUID) (*domainexpense.Expense, error) {
	entity, err := s.deps.Expenses.GetByID(ctx, id)
	if err != nil || entity.TripID != tc.Trip.ID {
		return nil, apperror.New("EXPENSE_NOT_FOUND")
	}
	return entity, nil
}

// validateCategory confirms the category is either a global default or belongs to this trip, so an
// expense can't be tagged with another trip's custom category.
func (s *service) validateCategory(ctx context.Context, tripID, categoryID uuid.UUID) error {
	if s.deps.Categories == nil {
		return nil
	}
	category, err := s.deps.Categories.GetByID(ctx, categoryID)
	if err != nil {
		return err
	}
	if category.TripID != nil && *category.TripID != tripID {
		return apperror.New("EXPENSE_CATEGORY_NOT_FOUND")
	}
	return nil
}

func (s *service) activeParticipants(ctx context.Context, tripID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.deps.Participants.ListByTripID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	result := make([]uuid.UUID, len(rows))
	for index := range rows {
		result[index] = rows[index].UserID
	}
	return result, nil
}

func (s *service) emit(ctx context.Context, eventType string, actorID uuid.UUID, tc tripctx.TripContext, entity *domainexpense.Expense) error {
	payerIDs := make([]uuid.UUID, len(entity.Payers))
	for i := range entity.Payers {
		payerIDs[i] = entity.Payers[i].UserID
	}
	splitIDs := make([]uuid.UUID, len(entity.Splits))
	for i := range entity.Splits {
		splitIDs[i] = entity.Splits[i].UserID
	}
	payload, err := json.Marshal(expenseevent.Changed{EventID: uuid.New(), TripID: tc.Trip.ID, TripCode: tc.Trip.Code,
		ExpenseID: entity.ID, Amount: entity.Amount.String(), Currency: entity.Currency, PayerIDs: payerIDs,
		SplitUserIDs: splitIDs, ActorID: actorID, OccurredAt: s.deps.Clock().UTC()})
	if err != nil {
		return err
	}
	return s.deps.Outbox.Create(ctx, &event.OutboxEvent{ID: uuid.New(), AggregateType: "expense", AggregateID: entity.ID,
		EventType: eventType, Payload: payload, Status: "pending", AvailableAt: s.deps.Clock().UTC(), CreatedAt: s.deps.Clock().UTC()})
}

func canEdit(actor uuid.UUID, tc tripctx.TripContext, creator uuid.UUID) bool {
	return tc.Participant.Role == domainparticipant.RolePlanner || tc.Trip.Settings.EditPermission == "everyone" || actor == creator
}

func validateDate(tc tripctx.TripContext, value time.Time) error {
	day := date(value)
	if day.Before(date(tc.Trip.StartDate).AddDate(0, 0, -7)) || day.After(date(tc.Trip.EndDate).AddDate(0, 0, 7)) {
		return apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "expense_date", Rule: "range", Message: "expense_date must be within seven days of the trip"}})
	}
	return nil
}

func date(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// source defaults to a hand-entered expense; a receipt conversion says so explicitly.
func source(value domainexpense.Source) domainexpense.Source {
	if value == "" {
		return domainexpense.SourceManual
	}
	return value
}
