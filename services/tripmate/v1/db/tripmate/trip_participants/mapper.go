package trip_participants

import (
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

func fromDomain(entity domainparticipant.Participant) Participant {
	model := Participant{ID: entity.ID, TripID: entity.TripID, UserID: entity.UserID, Role: string(entity.Role), JoinedAt: entity.JoinedAt, DisplayName: entity.DisplayName}
	if entity.BankInfo != nil {
		model.BankName = &entity.BankInfo.BankName
		model.BankAccountNumber = &entity.BankInfo.AccountNumber
		model.BankAccountHolder = &entity.BankInfo.AccountHolder
	}
	return model
}

func toDomain(model Participant) domainparticipant.Participant {
	entity := domainparticipant.Participant{ID: model.ID, TripID: model.TripID, UserID: model.UserID, Role: domainparticipant.Role(model.Role), DisplayName: model.DisplayName, JoinedAt: model.JoinedAt}
	if model.BankName != nil || model.BankAccountNumber != nil || model.BankAccountHolder != nil {
		entity.BankInfo = &domainparticipant.BankInfo{}
		if model.BankName != nil {
			entity.BankInfo.BankName = *model.BankName
		}
		if model.BankAccountNumber != nil {
			entity.BankInfo.AccountNumber = *model.BankAccountNumber
		}
		if model.BankAccountHolder != nil {
			entity.BankInfo.AccountHolder = *model.BankAccountHolder
		}
	}
	return entity
}

func participantRowToDomain(row participantWithUser) domainparticipant.Participant {
	entity := toDomain(row.Participant)
	entity.User = &domainuser.PublicUser{
		ID: row.UserID, Email: row.Email, Name: row.Name, AvatarURL: row.AvatarURL,
		CreatedAt: row.UserCreatedAt, UpdatedAt: row.UserUpdatedAt,
	}
	return entity
}
