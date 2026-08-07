package settlements

import (
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

func fromDomain(e domainsettlement.Settlement) Settlement {
	return Settlement{ID: e.ID, TripID: e.TripID, FromUserID: e.FromUserID, ToUserID: e.ToUserID, Amount: e.Amount, Currency: e.Currency, Method: string(e.Method), BankName: e.BankName, BankAccountNumber: e.BankAccountNumber, BankAccountHolder: e.BankAccountHolder, Note: e.Note, ProofURL: e.ProofURL, Status: string(e.Status), ApprovedByUserID: e.ApprovedByUserID, ApprovedAt: e.ApprovedAt, RejectedReason: e.RejectedReason, CreatedByUserID: e.CreatedByUserID, Version: e.Version, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}
func toDomain(m Settlement) domainsettlement.Settlement {
	e := domainsettlement.Settlement{ID: m.ID, TripID: m.TripID, FromUserID: m.FromUserID, ToUserID: m.ToUserID, Amount: m.Amount, Currency: m.Currency, Method: domainsettlement.Method(m.Method), BankName: m.BankName, BankAccountNumber: m.BankAccountNumber, BankAccountHolder: m.BankAccountHolder, Note: m.Note, ProofURL: m.ProofURL, Status: domainsettlement.Status(m.Status), ApprovedByUserID: m.ApprovedByUserID, ApprovedAt: m.ApprovedAt, RejectedReason: m.RejectedReason, CreatedByUserID: m.CreatedByUserID, Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	e.FromUser = &domainuser.PublicUser{ID: m.FromUserID, Email: m.FromEmail, Name: m.FromName, AvatarURL: m.FromAvatarURL}
	e.ToUser = &domainuser.PublicUser{ID: m.ToUserID, Email: m.ToEmail, Name: m.ToName, AvatarURL: m.ToAvatarURL}
	return e
}
