package money

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

type Money struct {
	Amount   decimal.Decimal
	Currency string
}

var Epsilon = decimal.NewFromFloat(0.01)

var zeroScaleCurrencies = map[string]struct{}{
	"IDR": {}, "JPY": {}, "KRW": {}, "VND": {},
}

func DisplayScale(currency string) int32 {
	if _, ok := zeroScaleCurrencies[strings.ToUpper(currency)]; ok {
		return 0
	}
	return 2
}

func Round(value decimal.Decimal, scale int32) decimal.Decimal {
	return value.RoundBank(scale)
}

func IsZero(value decimal.Decimal) bool { return value.Abs().LessThan(Epsilon) }

func Sum(values ...decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, value := range values {
		total = total.Add(value)
	}
	return total
}

func SplitEqual(total decimal.Decimal, n int, currency string) []decimal.Decimal {
	if n <= 0 {
		return []decimal.Decimal{}
	}
	weights := make([]decimal.Decimal, n)
	for i := range weights {
		weights[i] = decimal.NewFromInt(1)
	}
	return SplitProportional(total, weights, currency)
}

func SplitProportional(total decimal.Decimal, weights []decimal.Decimal, currency string) []decimal.Decimal {
	if len(weights) == 0 {
		return []decimal.Decimal{}
	}
	scale := DisplayScale(currency)
	factor := decimal.New(1, scale)
	rounded := Round(total, scale)
	negative := rounded.IsNegative()
	minorTotal := rounded.Abs().Mul(factor).IntPart()

	positiveWeights := make([]decimal.Decimal, len(weights))
	weightTotal := decimal.Zero
	for i, weight := range weights {
		if weight.IsPositive() {
			positiveWeights[i] = weight
			weightTotal = weightTotal.Add(weight)
		} else {
			positiveWeights[i] = decimal.Zero
		}
	}
	if weightTotal.IsZero() {
		for i := range positiveWeights {
			positiveWeights[i] = decimal.NewFromInt(1)
		}
		weightTotal = decimal.NewFromInt(int64(len(weights)))
	}

	type remainder struct {
		index    int
		fraction decimal.Decimal
	}
	minorParts := make([]int64, len(weights))
	remainders := make([]remainder, len(weights))
	allocated := int64(0)
	for i, weight := range positiveWeights {
		exact := decimal.NewFromInt(minorTotal).Mul(weight).Div(weightTotal)
		floor := exact.Floor().IntPart()
		minorParts[i] = floor
		allocated += floor
		remainders[i] = remainder{index: i, fraction: exact.Sub(decimal.NewFromInt(floor))}
	}
	sort.SliceStable(remainders, func(i, j int) bool {
		return remainders[i].fraction.GreaterThan(remainders[j].fraction)
	})
	for i := int64(0); i < minorTotal-allocated; i++ {
		minorParts[remainders[int(i)%len(remainders)].index]++
	}

	result := make([]decimal.Decimal, len(weights))
	for i, minor := range minorParts {
		result[i] = decimal.NewFromInt(minor).Div(factor)
		if negative {
			result[i] = result[i].Neg()
		}
	}
	return result
}
