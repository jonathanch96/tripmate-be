package participant

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

type Participant struct {
	ID       uuid.UUID                   `json:"id"`
	TripID   uuid.UUID                   `json:"trip_id"`
	UserID   uuid.UUID                   `json:"user_id"`
	Role     domainparticipant.Role      `json:"role"`
	JoinedAt time.Time                   `json:"joined_at"`
	BankInfo *domainparticipant.BankInfo `json:"bank_info"`
	User     *User                       `json:"user,omitempty"`
}

func FromDomain(entity domainparticipant.Participant) Participant {
	result := Participant{
		ID: entity.ID, TripID: entity.TripID, UserID: entity.UserID, Role: entity.Role,
		JoinedAt: entity.JoinedAt, BankInfo: entity.BankInfo,
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
