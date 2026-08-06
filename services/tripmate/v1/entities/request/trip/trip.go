package triprequest

type Create struct {
	Name                        string `json:"name" binding:"required,notblank,max=160"`
	BaseCurrency                string `json:"base_currency" binding:"required,len=3"`
	StartDate                   string `json:"start_date" binding:"required"`
	EndDate                     string `json:"end_date" binding:"required"`
	EditPermission              string `json:"edit_permission" binding:"required,oneof=everyone own_only"`
	ApprovalRequiredExpenses    bool   `json:"approval_required_expenses"`
	ApprovalRequiredSettlements bool   `json:"approval_required_settlements"`
	MultiCurrencyEnabled        bool   `json:"multi_currency_enabled"`
	AllowSettlementBeforeEnd    bool   `json:"allow_settlement_before_end"`
}

type Update struct {
	Name                        string `json:"name" binding:"required,notblank,max=160"`
	BaseCurrency                string `json:"base_currency" binding:"required,len=3"`
	EditPermission              string `json:"edit_permission" binding:"required,oneof=everyone own_only"`
	ApprovalRequiredExpenses    bool   `json:"approval_required_expenses"`
	ApprovalRequiredSettlements bool   `json:"approval_required_settlements"`
	MultiCurrencyEnabled        bool   `json:"multi_currency_enabled"`
	AllowSettlementBeforeEnd    bool   `json:"allow_settlement_before_end"`
	Version                     int    `json:"version" binding:"required,min=1"`
}
