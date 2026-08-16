package expenses

import (
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

func fromDomain(entity domainexpense.Expense) Expense {
	return Expense{
		ID: entity.ID, TripID: entity.TripID, CategoryID: entity.CategoryID, ExpenseDate: entity.ExpenseDate, Description: entity.Description,
		Amount: entity.Amount, Currency: entity.Currency, ChargedAmount: entity.ChargedAmount, ChargedCurrency: entity.ChargedCurrency,
		SplitType: string(entity.SplitType), Status: string(entity.Status),
		Source: string(entity.Source), Note: entity.Note, CreatedByUserID: entity.CreatedByUserID,
		ApprovedByUserID: entity.ApprovedByUserID, ApprovedAt: entity.ApprovedAt, RejectedReason: entity.RejectedReason,
		Version: entity.Version, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func toDomain(model Expense) domainexpense.Expense {
	result := domainexpense.Expense{
		ID: model.ID, TripID: model.TripID, CategoryID: model.CategoryID, ExpenseDate: model.ExpenseDate, Description: model.Description,
		Amount: model.Amount, Currency: model.Currency, ChargedAmount: model.ChargedAmount, ChargedCurrency: model.ChargedCurrency,
		SplitType: domainexpense.SplitType(model.SplitType), Status: domainexpense.Status(model.Status),
		Source: domainexpense.Source(model.Source), Note: model.Note, CreatedByUserID: model.CreatedByUserID,
		ApprovedByUserID: model.ApprovedByUserID, ApprovedAt: model.ApprovedAt, RejectedReason: model.RejectedReason,
		Version: model.Version, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
	result.CreatedBy = &domainuser.User{ID: model.CreatedByUserID, Email: model.CreatorEmail, Name: model.CreatorName, AvatarURL: model.CreatorAvatarURL, CreatedAt: model.CreatorCreatedAt, UpdatedAt: model.CreatorUpdatedAt}
	if model.ApprovedByUserID != nil && model.ApproverEmail != nil && model.ApproverName != nil {
		result.ApprovedBy = &domainuser.User{ID: *model.ApprovedByUserID, Email: *model.ApproverEmail, Name: *model.ApproverName, AvatarURL: model.ApproverAvatarURL}
		if model.ApproverCreatedAt != nil {
			result.ApprovedBy.CreatedAt = *model.ApproverCreatedAt
		}
		if model.ApproverUpdatedAt != nil {
			result.ApprovedBy.UpdatedAt = *model.ApproverUpdatedAt
		}
	}
	return result
}
