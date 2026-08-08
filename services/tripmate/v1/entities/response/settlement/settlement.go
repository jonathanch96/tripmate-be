package settlementresponse

import (
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	userresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/user"
)

type User = userresponse.User

type Settlement struct {
	ID         uuid.UUID `json:"id"`
	TripID     uuid.UUID `json:"trip_id"`
	FromUserID uuid.UUID `json:"from_user_id"`
	ToUserID   uuid.UUID `json:"to_user_id"`
	FromUser   *User     `json:"from_user"`
	ToUser     *User     `json:"to_user"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	Method     string    `json:"method"`
	Status     string    `json:"status"`
	// Bank details are the snapshot taken when the settlement was recorded, not a live read of the
	// payee's participant record. The account number is always masked on the way out.
	BankName          *string    `json:"bank_name"`
	BankAccountNumber *string    `json:"bank_account_number"`
	BankAccountHolder *string    `json:"bank_account_holder"`
	Note              *string    `json:"note"`
	ProofURL          *string    `json:"proof_url"`
	ApprovedBy        *uuid.UUID `json:"approved_by_user_id"`
	ApprovedAt        *time.Time `json:"approved_at"`
	RejectedReason    *string    `json:"rejected_reason"`
	Version           int        `json:"version"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func FromDomain(entity domainsettlement.Settlement) Settlement {
	result := Settlement{ID: entity.ID, TripID: entity.TripID, FromUserID: entity.FromUserID, ToUserID: entity.ToUserID,
		Amount: entity.Amount.StringFixedBank(money.DisplayScale(entity.Currency)), Currency: entity.Currency,
		Method: string(entity.Method), Status: string(entity.Status), BankName: entity.BankName,
		BankAccountNumber: maskAccount(entity.BankAccountNumber), BankAccountHolder: entity.BankAccountHolder,
		Note: entity.Note, ProofURL: entity.ProofURL, ApprovedBy: entity.ApprovedByUserID, ApprovedAt: entity.ApprovedAt,
		RejectedReason: entity.RejectedReason, Version: entity.Version, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
	result.FromUser, result.ToUser = publicUser(entity.FromUser), publicUser(entity.ToUser)
	return result
}

func ListFromDomain(entities []domainsettlement.Settlement) []Settlement {
	rows := make([]Settlement, len(entities))
	for i, entity := range entities {
		rows[i] = FromDomain(entity)
	}
	return rows
}

func publicUser(entity *domainuser.PublicUser) *User {
	if entity == nil {
		return nil
	}
	return &User{ID: entity.ID, Email: entity.Email, Name: entity.Name, AvatarURL: entity.AvatarURL,
		CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}

func maskAccount(value *string) *string {
	if value == nil {
		return nil
	}
	masked := domainparticipant.MaskAccount(*value)
	return &masked
}
