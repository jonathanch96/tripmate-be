package tripresponse

import (
	"github.com/google/uuid"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type Settings struct {
	EditPermission              string `json:"edit_permission"`
	ApprovalRequiredExpenses    bool   `json:"approval_required_expenses"`
	ApprovalRequiredSettlements bool   `json:"approval_required_settlements"`
	MultiCurrencyEnabled        bool   `json:"multi_currency_enabled"`
	AllowSettlementBeforeEnd    bool   `json:"allow_settlement_before_end"`
}

type Trip struct {
	ID              uuid.UUID `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	BaseCurrency    string    `json:"base_currency"`
	StartDate       string    `json:"start_date"`
	EndDate         string    `json:"end_date"`
	PlannerID       uuid.UUID `json:"planner_id"`
	IsFinalized     bool      `json:"is_finalized"`
	Settings        Settings  `json:"settings"`
	Version         int       `json:"version"`
	CanEditSettings bool      `json:"can_edit_settings"`
}

func FromDomain(entity domaintrip.Trip, canEditSettings bool) Trip {
	return Trip{
		ID: entity.ID, Code: entity.Code, Name: entity.Name, BaseCurrency: entity.BaseCurrency,
		StartDate: entity.StartDate.Format("2006-01-02"), EndDate: entity.EndDate.Format("2006-01-02"),
		PlannerID: entity.PlannerID, IsFinalized: entity.IsFinalized, Version: entity.Version,
		CanEditSettings: canEditSettings,
		Settings: Settings{
			EditPermission:              string(entity.Settings.EditPermission),
			ApprovalRequiredExpenses:    entity.Settings.ApprovalRequiredExpenses,
			ApprovalRequiredSettlements: entity.Settings.ApprovalRequiredSettlements,
			MultiCurrencyEnabled:        entity.Settings.MultiCurrencyEnabled,
			AllowSettlementBeforeEnd:    entity.Settings.AllowSettlementBeforeEnd,
		},
	}
}
