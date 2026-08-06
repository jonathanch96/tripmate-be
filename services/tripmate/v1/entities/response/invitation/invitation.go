package invitation

import (
	"time"

	"github.com/google/uuid"
	domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"
	participantresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/participant"
)

type Invitation struct {
	ID        uuid.UUID               `json:"id"`
	TripID    uuid.UUID               `json:"trip_id"`
	Email     string                  `json:"email"`
	Token     string                  `json:"token"`
	Status    domaininvitation.Status `json:"status"`
	ExpiresAt time.Time               `json:"expires_at"`
}

type InviteResult struct {
	Status      string                           `json:"status"`
	Invitation  *Invitation                      `json:"invitation,omitempty"`
	InviteLink  string                           `json:"invite_link,omitempty"`
	Participant *participantresponse.Participant `json:"participant,omitempty"`
}

func FromDomain(entity domaininvitation.Invitation) Invitation {
	return Invitation{ID: entity.ID, TripID: entity.TripID, Email: entity.Email, Token: entity.Token, Status: entity.Status, ExpiresAt: entity.ExpiresAt}
}

func FromDomains(entities []domaininvitation.Invitation) []Invitation {
	result := make([]Invitation, len(entities))
	for index, entity := range entities {
		result[index] = FromDomain(entity)
	}
	return result
}
