package expense_splits

import (
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

func fromDomain(entity domainexpense.Split) Split {
	return Split{ID: entity.ID, ExpenseID: entity.ExpenseID, UserID: entity.UserID, Amount: entity.Amount, Weight: entity.Weight, CreatedAt: entity.CreatedAt}
}

func toDomain(model Split) domainexpense.Split {
	return domainexpense.Split{ID: model.ID, ExpenseID: model.ExpenseID, UserID: model.UserID, Amount: model.Amount, Weight: model.Weight, CreatedAt: model.CreatedAt,
		User: &domainuser.User{ID: model.UserID, Email: model.UserEmail, Name: model.UserName, AvatarURL: model.UserAvatarURL, CreatedAt: model.UserCreatedAt, UpdatedAt: model.UserUpdatedAt}}
}
