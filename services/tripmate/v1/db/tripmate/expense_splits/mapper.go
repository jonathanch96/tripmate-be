package expense_splits

import domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"

func fromDomain(entity domainexpense.Split) Split {
	return Split{ID: entity.ID, ExpenseID: entity.ExpenseID, UserID: entity.UserID, Amount: entity.Amount, CreatedAt: entity.CreatedAt}
}

func toDomain(model Split) domainexpense.Split {
	return domainexpense.Split{ID: model.ID, ExpenseID: model.ExpenseID, UserID: model.UserID, Amount: model.Amount, CreatedAt: model.CreatedAt}
}
