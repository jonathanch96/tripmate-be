package expense_category

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
)

type repoStub struct {
	rows    map[uuid.UUID]domaincat.ExpenseCategory
	created *domaincat.ExpenseCategory
	deleted uuid.UUID
}

func newRepoStub() *repoStub { return &repoStub{rows: map[uuid.UUID]domaincat.ExpenseCategory{}} }

func (r *repoStub) ListForTrip(context.Context, uuid.UUID) ([]domaincat.ExpenseCategory, error) {
	return nil, nil
}

func (r *repoStub) GetByID(_ context.Context, id uuid.UUID) (*domaincat.ExpenseCategory, error) {
	row, ok := r.rows[id]
	if !ok {
		return nil, apperror.New("EXPENSE_CATEGORY_NOT_FOUND")
	}
	return &row, nil
}

func (r *repoStub) ExistsByName(_ context.Context, tripID *uuid.UUID, name string) (bool, error) {
	for _, row := range r.rows {
		if strings.EqualFold(row.Name, name) {
			if tripID == nil && row.TripID == nil {
				return true, nil
			}
			if tripID != nil && row.TripID != nil && *tripID == *row.TripID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *repoStub) Create(_ context.Context, entity *domaincat.ExpenseCategory) (*domaincat.ExpenseCategory, error) {
	r.rows[entity.ID] = *entity
	r.created = entity
	return entity, nil
}

func (r *repoStub) Delete(_ context.Context, id uuid.UUID) error {
	r.deleted = id
	delete(r.rows, id)
	return nil
}

func TestCreateRejectsDuplicateAgainstGlobalDefault(t *testing.T) {
	repo := newRepoStub()
	defaultID := uuid.New()
	repo.rows[defaultID] = domaincat.ExpenseCategory{ID: defaultID, Name: "Food & Drink", IsDefault: true}
	service := NewService(repo)
	tripID := uuid.New()
	if _, err := service.Create(context.Background(), tripID, "food & drink"); !apperror.Is(err, "EXPENSE_CATEGORY_ALREADY_EXISTS") {
		t.Fatalf("Create(dup of default) error = %v", err)
	}
}

func TestCreateAddsATripScopedCategory(t *testing.T) {
	repo := newRepoStub()
	service := NewService(repo)
	tripID := uuid.New()
	created, err := service.Create(context.Background(), tripID, "  Museums  ")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Museums" || created.TripID == nil || *created.TripID != tripID || created.IsDefault {
		t.Fatalf("created = %+v", created)
	}
}

func TestCreateRejectsBlankName(t *testing.T) {
	repo := newRepoStub()
	service := NewService(repo)
	if _, err := service.Create(context.Background(), uuid.New(), "   "); !apperror.Is(err, "VALIDATION_FAILED") {
		t.Fatalf("Create(blank) error = %v", err)
	}
}

func TestDeleteRefusesADefaultCategory(t *testing.T) {
	repo := newRepoStub()
	id := uuid.New()
	repo.rows[id] = domaincat.ExpenseCategory{ID: id, Name: "Other", IsDefault: true}
	service := NewService(repo)
	if err := service.Delete(context.Background(), uuid.New(), id); !apperror.Is(err, "EXPENSE_CATEGORY_IS_DEFAULT") {
		t.Fatalf("Delete(default) error = %v", err)
	}
}

func TestDeleteRefusesAnotherTripsCategory(t *testing.T) {
	repo := newRepoStub()
	id, owner, actor := uuid.New(), uuid.New(), uuid.New()
	repo.rows[id] = domaincat.ExpenseCategory{ID: id, TripID: &owner, Name: "Souvenirs"}
	service := NewService(repo)
	if err := service.Delete(context.Background(), actor, id); !apperror.Is(err, "EXPENSE_CATEGORY_NOT_FOUND") {
		t.Fatalf("Delete(other trip) error = %v", err)
	}
}

func TestDeleteRemovesOwnCustomCategory(t *testing.T) {
	repo := newRepoStub()
	id, tripID := uuid.New(), uuid.New()
	repo.rows[id] = domaincat.ExpenseCategory{ID: id, TripID: &tripID, Name: "Souvenirs"}
	service := NewService(repo)
	if err := service.Delete(context.Background(), tripID, id); err != nil {
		t.Fatal(err)
	}
	if repo.deleted != id {
		t.Fatalf("deleted = %s, want %s", repo.deleted, id)
	}
}
