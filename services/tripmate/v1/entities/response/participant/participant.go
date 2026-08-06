package participantresponse

import (
	"time"

	"github.com/google/uuid"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
}

type BankInfo struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountHolder string `json:"account_holder"`
}

type Participant struct {
	ID       uuid.UUID `json:"id"`
	TripID   uuid.UUID `json:"trip_id"`
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	BankInfo *BankInfo `json:"bank_info"`
	User     *User     `json:"user,omitempty"`
}

func FromDomain(entity domainparticipant.Participant) Participant {
	result := Participant{
		ID: entity.ID, TripID: entity.TripID, UserID: entity.UserID, Role: string(entity.Role),
		JoinedAt: entity.JoinedAt,
	}
	if entity.BankInfo != nil {
		result.BankInfo = &BankInfo{BankName: entity.BankInfo.BankName, AccountNumber: entity.BankInfo.AccountNumber, AccountHolder: entity.BankInfo.AccountHolder}
	}
	if entity.User != nil {
		result.User = &User{ID: entity.User.ID, Name: entity.User.Name, Email: entity.User.Email, AvatarURL: entity.User.AvatarURL}
	}
	return result
}

func FromDomains(entities []domainparticipant.Participant) []Participant {
	result := make([]Participant, len(entities))
	for index, entity := range entities {
		result[index] = FromDomain(entity)
	}
	return result
}
