package settlement

import (
	"time"

	"github.com/google/uuid"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
)

type Method string
type Status string

const (
	MethodCash         Method = "cash"
	MethodBankTransfer Method = "bank_transfer"

	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

type Settlement struct {
	ID, TripID, FromUserID, ToUserID, CreatedByUserID uuid.UUID
	Amount                                            decimal.Decimal
	Currency                                          string
	Method                                            Method
	BankName, BankAccountNumber, BankAccountHolder    *string
	Note, ProofURL                                    *string
	Status                                            Status
	ApprovedByUserID                                  *uuid.UUID
	ApprovedAt                                        *time.Time
	RejectedReason                                    *string
	Version                                           int
	FromUser, ToUser                                  *domainuser.PublicUser
	CreatedAt, UpdatedAt                              time.Time
}
