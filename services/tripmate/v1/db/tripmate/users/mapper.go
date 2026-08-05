package users

import domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"

func toDomain(model User) domainuser.User {
	return domainuser.User{
		ID: model.ID, Email: model.Email, Name: model.Name, PasswordHash: model.PasswordHash,
		AvatarURL: cloneString(model.AvatarURL), CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}

func fromDomain(entity domainuser.User) User {
	return User{
		ID: entity.ID, Email: entity.Email, Name: entity.Name, PasswordHash: entity.PasswordHash,
		AvatarURL: cloneString(entity.AvatarURL), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
