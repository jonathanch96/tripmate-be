package expensecategoryresponse

import (
	"github.com/google/uuid"
	domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"
)

type ExpenseCategory struct {
	ID        uuid.UUID  `json:"id"`
	TripID    *uuid.UUID `json:"trip_id"`
	Name      string     `json:"name"`
	IsDefault bool       `json:"is_default"`
}

func FromDomain(entity domaincat.ExpenseCategory) ExpenseCategory {
	return ExpenseCategory{ID: entity.ID, TripID: entity.TripID, Name: entity.Name, IsDefault: entity.IsDefault}
}

func FromDomains(entities []domaincat.ExpenseCategory) []ExpenseCategory {
	result := make([]ExpenseCategory, len(entities))
	for index, entity := range entities {
		result[index] = FromDomain(entity)
	}
	return result
}
