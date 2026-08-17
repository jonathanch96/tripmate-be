package tripresponse

import (
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
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
	ID              uuid.UUID  `json:"id"`
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	BaseCurrency    string     `json:"base_currency"`
	Country         *string    `json:"country,omitempty"`
	StartDate       string     `json:"start_date"`
	EndDate         string     `json:"end_date"`
	PlannerID       uuid.UUID  `json:"planner_id"`
	IsFinalized     bool       `json:"is_finalized"`
	IsArchived      bool       `json:"is_archived"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	Settings        Settings   `json:"settings"`
	Version         int        `json:"version"`
	CanEditSettings bool       `json:"can_edit_settings"`
	// Currencies, MemberCount and TotalSpend are only populated on the trip list endpoint. Currencies
	// is every currency actually used on the trip, base currency first (omitted, not just empty,
	// elsewhere). MemberCount and TotalSpend are omitted (zero-valued) elsewhere too.
	Currencies  []string `json:"currencies,omitempty"`
	MemberCount int      `json:"member_count,omitempty"`
	TotalSpend  *string  `json:"total_spend,omitempty"`
}

func FromDomain(entity domaintrip.Trip, canEditSettings bool) Trip {
	result := Trip{
		ID: entity.ID, Code: entity.Code, Name: entity.Name, BaseCurrency: entity.BaseCurrency, Country: entity.Country,
		StartDate: entity.StartDate.Format("2006-01-02"), EndDate: entity.EndDate.Format("2006-01-02"),
		PlannerID: entity.PlannerID, IsFinalized: entity.IsFinalized, Version: entity.Version,
		IsArchived: entity.IsArchived, ArchivedAt: entity.ArchivedAt,
		CanEditSettings: canEditSettings, Currencies: entity.Currencies, MemberCount: entity.MemberCount,
		Settings: Settings{
			EditPermission:              string(entity.Settings.EditPermission),
			ApprovalRequiredExpenses:    entity.Settings.ApprovalRequiredExpenses,
			ApprovalRequiredSettlements: entity.Settings.ApprovalRequiredSettlements,
			MultiCurrencyEnabled:        entity.Settings.MultiCurrencyEnabled,
			AllowSettlementBeforeEnd:    entity.Settings.AllowSettlementBeforeEnd,
		},
	}
	if entity.TotalSpend != nil {
		value := entity.TotalSpend.StringFixedBank(money.DisplayScale(entity.BaseCurrency))
		result.TotalSpend = &value
	}
	return result
}
