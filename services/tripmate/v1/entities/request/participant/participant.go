package participant

import (
	"github.com/google/uuid"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
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
	BankInfo *BankInfo              `json:"bank_info"`
	Role     *domainparticipant.Role `json:"role" binding:"omitempty,oneof=planner participant"`
}
