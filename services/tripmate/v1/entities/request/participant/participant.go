package participantrequest

import (
	"github.com/google/uuid"
)

type Add struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

type BankInfo struct {
	BankName      string `json:"bank_name" binding:"required,notblank,max=120"`
	AccountNumber string `json:"account_number" binding:"required,notblank,max=120"`
	AccountHolder string `json:"account_holder" binding:"required,notblank,max=160"`
}

type Update struct {
	BankInfo    *BankInfo `json:"bank_info"`
	Role        *string   `json:"role" binding:"omitempty,oneof=planner participant"`
	DisplayName *string   `json:"display_name" binding:"omitempty,max=120"`
}
