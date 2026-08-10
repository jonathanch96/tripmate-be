package expense_category

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
)

type service struct{ repo Repository }

func NewService(repo Repository) Service { return &service{repo: repo} }

func (s *service) List(ctx context.Context, tripID uuid.UUID) ([]domaincat.ExpenseCategory, error) {
	return s.repo.ListForTrip(ctx, tripID)
}

// Create adds a custom category for the trip. Names are de-duplicated case-insensitively against
// both this trip's custom categories and the global defaults, so "food & drink" can't shadow "Food & Drink".
func (s *service) Create(ctx context.Context, tripID uuid.UUID, name string) (*domaincat.ExpenseCategory, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 50 {
		return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "name", Rule: "required", Message: "name must be 1-50 characters"}})
	}
	tripScoped, err := s.repo.ExistsByName(ctx, &tripID, name)
	if err != nil {
		return nil, err
	}
	global, err := s.repo.ExistsByName(ctx, nil, name)
	if err != nil {
		return nil, err
	}
	if tripScoped || global {
		return nil, apperror.New("EXPENSE_CATEGORY_ALREADY_EXISTS")
	}
	return s.repo.Create(ctx, &domaincat.ExpenseCategory{ID: uuid.New(), TripID: &tripID, Name: name})
}

// Delete refuses to remove a global default and refuses to remove another trip's custom category.
func (s *service) Delete(ctx context.Context, tripID uuid.UUID, id uuid.UUID) error {
	category, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if category.TripID == nil || category.IsDefault {
		return apperror.New("EXPENSE_CATEGORY_IS_DEFAULT")
	}
	if *category.TripID != tripID {
		return apperror.New("EXPENSE_CATEGORY_NOT_FOUND")
	}
	return s.repo.Delete(ctx, id)
}
