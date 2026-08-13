package trips

import domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"

func fromDomain(entity domaintrip.Trip) Trip {
	return Trip{
		ID: entity.ID, Code: entity.Code, Name: entity.Name, BaseCurrency: entity.BaseCurrency,
		StartDate: entity.StartDate, EndDate: entity.EndDate, PlannerID: entity.PlannerID,
		IsFinalized: entity.IsFinalized, FinalizedAt: entity.FinalizedAt,
		IsArchived: entity.IsArchived, ArchivedAt: entity.ArchivedAt,
		EditPermission:      string(entity.Settings.EditPermission),
		ApprovalExpenses:    entity.Settings.ApprovalRequiredExpenses,
		ApprovalSettlements: entity.Settings.ApprovalRequiredSettlements,
		MultiCurrency:       entity.Settings.MultiCurrencyEnabled,
		AllowSettlement:     entity.Settings.AllowSettlementBeforeEnd,
		Version:             entity.Version, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func toDomain(model Trip) domaintrip.Trip {
	return domaintrip.Trip{
		ID: model.ID, Code: model.Code, Name: model.Name, BaseCurrency: model.BaseCurrency,
		StartDate: model.StartDate, EndDate: model.EndDate, PlannerID: model.PlannerID,
		IsFinalized: model.IsFinalized, FinalizedAt: model.FinalizedAt,
		IsArchived: model.IsArchived, ArchivedAt: model.ArchivedAt,
		Settings: domaintrip.Settings{
			EditPermission:              domaintrip.EditPermission(model.EditPermission),
			ApprovalRequiredExpenses:    model.ApprovalExpenses,
			ApprovalRequiredSettlements: model.ApprovalSettlements,
			MultiCurrencyEnabled:        model.MultiCurrency,
			AllowSettlementBeforeEnd:    model.AllowSettlement,
		},
		Version: model.Version, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}
