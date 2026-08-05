package trip_invitations

import domaininvitation "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/invitation"

func fromDomain(entity domaininvitation.Invitation) Invitation {
	return Invitation{
		ID: entity.ID, TripID: entity.TripID, Email: entity.Email, Token: entity.Token,
		Status: string(entity.Status), InvitedBy: entity.InvitedBy, ExpiresAt: entity.ExpiresAt,
		AcceptedAt: entity.AcceptedAt, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func toDomain(model Invitation) domaininvitation.Invitation {
	return domaininvitation.Invitation{
		ID: model.ID, TripID: model.TripID, Email: model.Email, Token: model.Token,
		Status: domaininvitation.Status(model.Status), InvitedBy: model.InvitedBy,
		ExpiresAt: model.ExpiresAt, AcceptedAt: model.AcceptedAt,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}
