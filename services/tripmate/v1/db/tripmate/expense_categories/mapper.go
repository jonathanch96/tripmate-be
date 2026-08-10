package expense_categories

import domaincat "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense_category"

func fromDomain(entity domaincat.ExpenseCategory) ExpenseCategory {
	return ExpenseCategory{ID: entity.ID, TripID: entity.TripID, Name: entity.Name, IsDefault: entity.IsDefault,
		CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}

func toDomain(model ExpenseCategory) domaincat.ExpenseCategory {
	return domaincat.ExpenseCategory{ID: model.ID, TripID: model.TripID, Name: model.Name, IsDefault: model.IsDefault,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}
