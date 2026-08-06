package expenseresponse

import (
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	userresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/user"
)

type User = userresponse.User

type Payer struct {
	UserID uuid.UUID `json:"user_id"`
	User   *User     `json:"user"`
	Amount string    `json:"amount"`
}

type Split struct {
	UserID uuid.UUID `json:"user_id"`
	User   *User     `json:"user"`
	Amount string    `json:"amount"`
}

type Expense struct {
	ID             uuid.UUID  `json:"id"`
	TripID         uuid.UUID  `json:"trip_id"`
	ExpenseDate    string     `json:"expense_date"`
	Description    string     `json:"description"`
	Amount         string     `json:"amount"`
	Currency       string     `json:"currency"`
	SplitType      string     `json:"split_type"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	Note           *string    `json:"note"`
	Payers         []Payer    `json:"payers"`
	Splits         []Split    `json:"splits"`
	CreatedBy      *User      `json:"created_by"`
	ApprovedBy     *User      `json:"approved_by"`
	ApprovedAt     *time.Time `json:"approved_at"`
	RejectedReason *string    `json:"rejected_reason"`
	CanEdit        bool       `json:"can_edit"`
	CanDelete      bool       `json:"can_delete"`
	CanApprove     bool       `json:"can_approve"`
	CanReject      bool       `json:"can_reject"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func FromDomain(entity domainexpense.Expense, tc tripctx.TripContext, actorID uuid.UUID) Expense {
	result := Expense{ID: entity.ID, TripID: entity.TripID, ExpenseDate: entity.ExpenseDate.Format("2006-01-02"),
		Description: entity.Description, Amount: entity.Amount.StringFixedBank(displayScale(entity.Currency)), Currency: entity.Currency,
		SplitType: string(entity.SplitType), Status: string(entity.Status), Source: string(entity.Source), Note: entity.Note,
		ApprovedAt: entity.ApprovedAt, RejectedReason: entity.RejectedReason, Version: entity.Version,
		CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
	allowed := tc.Participant.Role == domainparticipant.RolePlanner || tc.Trip.Settings.EditPermission == "everyone" || entity.CreatedByUserID == actorID
	result.CanEdit, result.CanDelete = allowed, allowed
	mayReview := tc.Participant.Role == domainparticipant.RolePlanner && entity.Status == domainexpense.StatusPending
	result.CanApprove, result.CanReject = mayReview, mayReview
	if entity.CreatedBy != nil {
		value := userresponse.FromDomain(*entity.CreatedBy)
		result.CreatedBy = &value
	}
	if entity.ApprovedBy != nil {
		value := userresponse.FromDomain(*entity.ApprovedBy)
		result.ApprovedBy = &value
	}
	result.Payers = make([]Payer, len(entity.Payers))
	for i, row := range entity.Payers {
		result.Payers[i] = Payer{UserID: row.UserID, Amount: row.Amount.StringFixedBank(displayScale(entity.Currency))}
		if row.User != nil {
			value := userresponse.FromDomain(*row.User)
			result.Payers[i].User = &value
		}
	}
	result.Splits = make([]Split, len(entity.Splits))
	for i, row := range entity.Splits {
		result.Splits[i] = Split{UserID: row.UserID, Amount: row.Amount.StringFixedBank(displayScale(entity.Currency))}
		if row.User != nil {
			value := userresponse.FromDomain(*row.User)
			result.Splits[i].User = &value
		}
	}
	return result
}

func displayScale(currency string) int32 {
	switch currency {
	case "IDR", "JPY", "KRW", "VND":
		return 0
	default:
		return 2
	}
}
