package userresponse

import (
	"time"

	"github.com/google/uuid"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type User struct {
	ID         uuid.UUID `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	AvatarURL  *string   `json:"avatar_url"`
	HasAccount bool      `json:"has_account"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func FromDomain(entity domainuser.User) User {
	public := entity.Public()
	return User{ID: public.ID, Email: public.Email, Name: public.Name, AvatarURL: public.AvatarURL,
		HasAccount: public.HasAccount, CreatedAt: public.CreatedAt, UpdatedAt: public.UpdatedAt}
}

type Lookup struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

func LookupFromDomain(entity domainuser.User) Lookup {
	return Lookup{ID: entity.ID, Name: entity.Name, Email: entity.Email}
}
