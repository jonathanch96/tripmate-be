package expense_payers

import domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"

func fromDomain(entity domainexpense.Payer) Payer {
	return Payer{ID: entity.ID, ExpenseID: entity.ExpenseID, UserID: entity.UserID, Amount: entity.Amount, CreatedAt: entity.CreatedAt}
}

func toDomain(model Payer) domainexpense.Payer {
	return domainexpense.Payer{ID: model.ID, ExpenseID: model.ExpenseID, UserID: model.UserID, Amount: model.Amount, CreatedAt: model.CreatedAt}
}
