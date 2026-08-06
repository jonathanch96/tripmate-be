package expense

import (
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/money"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"github.com/shopspring/decimal"
)

var ErrNotImplemented = errors.New("item split calculation is not implemented")

type ItemAssignment struct{}

type SplitInput struct {
	Amount       decimal.Decimal
	Currency     string
	SplitType    domainexpense.SplitType
	Participants []uuid.UUID
	Manual       map[uuid.UUID]decimal.Decimal
	Items        []ItemAssignment
}

func CalculateSplits(input SplitInput) ([]domainexpense.Split, error) {
	switch input.SplitType {
	case domainexpense.SplitEqual:
		if len(input.Participants) == 0 || hasDuplicate(input.Participants) {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		participants := append([]uuid.UUID(nil), input.Participants...)
		sort.Slice(participants, func(i, j int) bool { return participants[i].String() < participants[j].String() })
		amounts := money.SplitEqual(input.Amount, len(participants), input.Currency)
		result := make([]domainexpense.Split, len(participants))
		for index := range participants {
			result[index] = domainexpense.Split{UserID: participants[index], Amount: amounts[index]}
		}
		return result, nil
	case domainexpense.SplitManual:
		if len(input.Manual) == 0 {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		participants := make([]uuid.UUID, 0, len(input.Manual))
		for userID, value := range input.Manual {
			if value.IsNegative() {
				return nil, apperror.New("VALIDATION_FAILED")
			}
			participants = append(participants, userID)
		}
		sort.Slice(participants, func(i, j int) bool { return participants[i].String() < participants[j].String() })
		result := make([]domainexpense.Split, len(participants))
		values := make([]decimal.Decimal, len(participants))
		for index, userID := range participants {
			values[index] = input.Manual[userID]
			result[index] = domainexpense.Split{UserID: userID, Amount: values[index]}
		}
		if !sum(values).RoundBank(displayScale(input.Currency)).Equal(input.Amount.RoundBank(displayScale(input.Currency))) {
			message := fmt.Sprintf("splits sum to %s; expected %s", sum(values).String(), input.Amount.String())
			return nil, apperror.WithFields("SPLIT_SUM_MISMATCH", []apperror.FieldError{{Field: "splits", Rule: "sum", Message: message}})
		}
		return result, nil
	case domainexpense.SplitItem:
		return nil, ErrNotImplemented
	default:
		return nil, apperror.New("VALIDATION_FAILED")
	}
}

func ValidatePayers(amount decimal.Decimal, currency string, payers []domainexpense.Payer) error {
	if len(payers) == 0 {
		return apperror.New("VALIDATION_FAILED")
	}
	seen := make(map[uuid.UUID]struct{}, len(payers))
	values := make([]decimal.Decimal, len(payers))
	for index, payer := range payers {
		if payer.Amount.LessThanOrEqual(decimal.Zero) {
			return apperror.New("VALIDATION_FAILED")
		}
		if _, exists := seen[payer.UserID]; exists {
			return apperror.New("VALIDATION_FAILED")
		}
		seen[payer.UserID] = struct{}{}
		values[index] = payer.Amount
	}
	if !sum(values).RoundBank(displayScale(currency)).Equal(amount.RoundBank(displayScale(currency))) {
		message := fmt.Sprintf("payers sum to %s; expected %s", sum(values).String(), amount.String())
		return apperror.WithFields("PAYER_SUM_MISMATCH", []apperror.FieldError{{Field: "payers", Rule: "sum", Message: message}})
	}
	return nil
}

func ValidateExpenseParticipants(active []uuid.UUID, payers []domainexpense.Payer, splits []domainexpense.Split) error {
	allowed := make(map[uuid.UUID]struct{}, len(active))
	for _, userID := range active {
		allowed[userID] = struct{}{}
	}
	for _, payer := range payers {
		if _, exists := allowed[payer.UserID]; !exists {
			return apperror.New("VALIDATION_FAILED")
		}
	}
	for _, split := range splits {
		if _, exists := allowed[split.UserID]; !exists {
			return apperror.New("VALIDATION_FAILED")
		}
	}
	return nil
}

func hasDuplicate(values []uuid.UUID) bool {
	seen := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func sum(values []decimal.Decimal) decimal.Decimal { return money.Sum(values...) }
func displayScale(currency string) int32           { return money.DisplayScale(currency) }
