package expenserequest

type Payer struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Amount string `json:"amount" binding:"required"`
}

type Split struct {
	UserID string  `json:"user_id" binding:"required,uuid"`
	Amount string  `json:"amount" binding:"required"`
	Weight *string `json:"weight"`
}

type Create struct {
	ExpenseDate string `json:"expense_date" binding:"required,datetime=2006-01-02"`
	Description string `json:"description" binding:"required,max=255"`
	Amount      string `json:"amount" binding:"required"`
	Currency    string `json:"currency" binding:"required,len=3"`
	// ChargedAmount and ChargedCurrency are an optional record of what the payer's card or account
	// actually showed. ChargedAmount is only read when ChargedCurrency is also set.
	ChargedAmount   *string  `json:"charged_amount"`
	ChargedCurrency *string  `json:"charged_currency" binding:"omitempty,len=3"`
	CategoryID      *string  `json:"category_id" binding:"omitempty,uuid"`
	SplitType       string   `json:"split_type" binding:"required,oneof=equal manual item percent shares"`
	Payers          []Payer  `json:"payers" binding:"required,min=1,dive"`
	Participants    []string `json:"participants"`
	Splits          []Split  `json:"splits"`
	Note            *string  `json:"note"`
}

type Update struct {
	ExpenseDate *string `json:"expense_date"`
	Description *string `json:"description"`
	Amount      *string `json:"amount"`
	Currency    *string `json:"currency"`
	// ChargedAmount and ChargedCurrency follow the same convention as CategoryID: omitted means
	// "leave unchanged", a non-empty ChargedCurrency sets both fields, and an explicit empty-string
	// ChargedCurrency clears both back to unset.
	ChargedAmount   *string  `json:"charged_amount"`
	ChargedCurrency *string  `json:"charged_currency"`
	CategoryID      *string  `json:"category_id" binding:"omitempty,uuid"`
	SplitType       *string  `json:"split_type"`
	Payers          []Payer  `json:"payers"`
	Participants    []string `json:"participants"`
	Splits          []Split  `json:"splits"`
	Note            *string  `json:"note"`
	Version         int      `json:"version" binding:"required,min=1"`
}

type Reject struct {
	Reason string `json:"reason" binding:"required,max=500"`
}
