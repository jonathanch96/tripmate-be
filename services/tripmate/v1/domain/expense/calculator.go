package expense

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/money"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	"github.com/shopspring/decimal"
)

// ItemAssignment is one line of a bill together with everyone sharing it. A line shared by several
// people is the normal case, not the exception.
type ItemAssignment struct {
	Amount  decimal.Decimal
	UserIDs []uuid.UUID
}

type SplitInput struct {
	Amount       decimal.Decimal
	Currency     string
	SplitType    domainexpense.SplitType
	Participants []uuid.UUID
	Manual       map[uuid.UUID]decimal.Decimal
	// Weights carries percentages (0-100, summing to 100) for a percent split, or arbitrary positive
	// share counts for a shares split. Only read for SplitPercent/SplitShares.
	Weights map[uuid.UUID]decimal.Decimal
	Items   []ItemAssignment
	// Extras is tax plus service charge, allocated across diners in proportion to what they ate.
	// Only meaningful for an item split.
	Extras decimal.Decimal
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
		return splitByItem(input)
	case domainexpense.SplitPercent:
		return splitByPercent(input)
	case domainexpense.SplitShares:
		return splitByShares(input)
	default:
		return nil, apperror.New("VALIDATION_FAILED")
	}
}

// splitByPercent distributes the amount in proportion to each participant's declared percentage,
// which must sum to 100 so the split reflects exactly what the caller asked for.
func splitByPercent(input SplitInput) ([]domainexpense.Split, error) {
	participants, weights, err := sortedWeights(input.Weights)
	if err != nil {
		return nil, err
	}
	total := sum(weights)
	if !total.RoundBank(2).Equal(decimal.NewFromInt(100)) {
		message := fmt.Sprintf("percentages sum to %s; expected 100", total.String())
		return nil, apperror.WithFields("SPLIT_PERCENT_MISMATCH", []apperror.FieldError{{Field: "splits", Rule: "sum", Message: message}})
	}
	return weightedSplits(input, participants, weights), nil
}

// splitByShares distributes the amount in proportion to each participant's declared share count.
// Shares have no fixed total to sum to - only the ratio between them matters.
func splitByShares(input SplitInput) ([]domainexpense.Split, error) {
	participants, weights, err := sortedWeights(input.Weights)
	if err != nil {
		return nil, err
	}
	return weightedSplits(input, participants, weights), nil
}

func sortedWeights(weights map[uuid.UUID]decimal.Decimal) ([]uuid.UUID, []decimal.Decimal, error) {
	if len(weights) == 0 {
		return nil, nil, apperror.New("VALIDATION_FAILED")
	}
	participants := make([]uuid.UUID, 0, len(weights))
	for userID, weight := range weights {
		if !weight.IsPositive() {
			return nil, nil, apperror.New("VALIDATION_FAILED")
		}
		participants = append(participants, userID)
	}
	sort.Slice(participants, func(i, j int) bool { return participants[i].String() < participants[j].String() })
	values := make([]decimal.Decimal, len(participants))
	for index, userID := range participants {
		values[index] = weights[userID]
	}
	return participants, values, nil
}

func weightedSplits(input SplitInput, participants []uuid.UUID, weights []decimal.Decimal) []domainexpense.Split {
	amounts := money.SplitProportional(input.Amount, weights, input.Currency)
	result := make([]domainexpense.Split, len(participants))
	for index, userID := range participants {
		weight := weights[index]
		result[index] = domainexpense.Split{UserID: userID, Amount: amounts[index], Weight: &weight}
	}
	return result
}

// splitByItem charges each line to the people who shared it, then spreads tax and service across
// them in proportion to what they ate. Every step allocates through money's exact-sum helpers, so
// the resulting splits add up to the bill to the cent rather than to within a rounding error.
func splitByItem(input SplitInput) ([]domainexpense.Split, error) {
	if len(input.Items) == 0 {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	subtotals := make(map[uuid.UUID]decimal.Decimal)
	itemsTotal := decimal.Zero
	for _, item := range input.Items {
		if len(item.UserIDs) == 0 {
			// F-05/A-3: an unassigned line must be resolved before it can reach a split.
			return nil, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "items", Rule: "assigned", Message: "every item must be assigned to at least one participant"}})
		}
		if item.Amount.IsNegative() {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		assignees := append([]uuid.UUID(nil), item.UserIDs...)
		sort.Slice(assignees, func(i, j int) bool { return assignees[i].String() < assignees[j].String() })
		if hasDuplicate(assignees) {
			return nil, apperror.New("VALIDATION_FAILED")
		}
		shares := money.SplitEqual(item.Amount, len(assignees), input.Currency)
		for index, userID := range assignees {
			subtotals[userID] = subtotals[userID].Add(shares[index])
		}
		itemsTotal = itemsTotal.Add(item.Amount)
	}

	diners := make([]uuid.UUID, 0, len(subtotals))
	for userID := range subtotals {
		diners = append(diners, userID)
	}
	sort.Slice(diners, func(i, j int) bool { return diners[i].String() < diners[j].String() })

	weights := make([]decimal.Decimal, len(diners))
	for index, userID := range diners {
		weights[index] = subtotals[userID]
	}
	extras := input.Extras
	if extras.IsNegative() {
		return nil, apperror.New("VALIDATION_FAILED")
	}
	allocated := money.SplitProportional(extras, weights, input.Currency)

	result := make([]domainexpense.Split, len(diners))
	values := make([]decimal.Decimal, len(diners))
	for index, userID := range diners {
		values[index] = subtotals[userID].Add(allocated[index])
		result[index] = domainexpense.Split{UserID: userID, Amount: values[index]}
	}

	// The splits must reconstruct the bill exactly. Amount is the authority when the caller supplies
	// one, so a mismatch is a real inconsistency rather than a rounding artifact.
	scale := displayScale(input.Currency)
	expected := itemsTotal.Add(extras)
	if input.Amount.IsPositive() {
		expected = input.Amount
	}
	if !sum(values).RoundBank(scale).Equal(expected.RoundBank(scale)) {
		message := fmt.Sprintf("splits sum to %s; expected %s", sum(values).String(), expected.String())
		return nil, apperror.WithFields("SPLIT_SUM_MISMATCH", []apperror.FieldError{{Field: "splits", Rule: "sum", Message: message}})
	}
	return result, nil
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
