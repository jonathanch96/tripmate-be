package balance

import (
	"bytes"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	"github.com/shopspring/decimal"
)

type position struct {
	userID uuid.UUID
	amount decimal.Decimal
}

func Optimize(balances []domainbalance.ParticipantBalance, currency string) []domainbalance.Transfer {
	debtors, creditors := make([]position, 0, len(balances)), make([]position, 0, len(balances))
	for _, balance := range balances {
		if money.IsZero(balance.NetBalance) {
			continue
		}
		position := position{userID: balance.UserID, amount: balance.NetBalance.Abs()}
		if balance.NetBalance.IsNegative() {
			debtors = append(debtors, position)
		} else {
			creditors = append(creditors, position)
		}
	}
	result := make([]domainbalance.Transfer, 0, len(balances)-1)
	for iteration := 0; len(debtors) > 0 && len(creditors) > 0 && iteration < 2*len(balances); iteration++ {
		sortPositions(debtors)
		sortPositions(creditors)
		amount := decimal.Min(debtors[0].amount, creditors[0].amount)
		if money.IsZero(amount) {
			break
		}
		result = append(result, domainbalance.Transfer{FromUserID: debtors[0].userID, ToUserID: creditors[0].userID, Amount: amount, Currency: strings.ToUpper(currency)})
		debtors[0].amount = debtors[0].amount.Sub(amount)
		creditors[0].amount = creditors[0].amount.Sub(amount)
		if money.IsZero(debtors[0].amount) {
			debtors = debtors[1:]
		}
		if money.IsZero(creditors[0].amount) {
			creditors = creditors[1:]
		}
	}
	return result
}

func sortPositions(rows []position) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].amount.Equal(rows[j].amount) {
			return bytes.Compare(rows[i].userID[:], rows[j].userID[:]) < 0
		}
		return rows[i].amount.GreaterThan(rows[j].amount)
	})
}
