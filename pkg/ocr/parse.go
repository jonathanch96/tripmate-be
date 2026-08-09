// Package ocr turns a photograph of a bill into line items.
//
// The parsing lives apart from any particular provider because model output is the least reliable
// input this system takes: it arrives fenced in markdown, wrapped in prose, with amounts as numbers
// one day and strings the next. Everything defensive lives here and is covered by recorded fixtures.
package ocr

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/money"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
	"github.com/shopspring/decimal"
)

// payload mirrors the JSON shape asked of the model. Amounts are json.RawMessage because the model
// cannot be trusted to pick a side between 12.5 and "12.50".
type payload struct {
	Merchant  string `json:"merchant"`
	Date      string `json:"date"`
	Currency  string `json:"currency"`
	LineItems []struct {
		Name       string          `json:"name"`
		Quantity   json.RawMessage `json:"quantity"`
		UnitPrice  json.RawMessage `json:"unitPrice"`
		TotalPrice json.RawMessage `json:"totalPrice"`
	} `json:"lineItems"`
	Subtotal      json.RawMessage `json:"subtotal"`
	Tax           json.RawMessage `json:"tax"`
	ServiceCharge json.RawMessage `json:"serviceCharge"`
	Total         json.RawMessage `json:"total"`
}

// Parse converts one model response into a result. It fails only when there is no JSON object to be
// found at all (rule OCR-2); every lesser problem degrades to a flagged field so a human can fix a
// line instead of losing the whole receipt.
func Parse(raw []byte) (*receiptdomain.ExtractionResult, error) {
	document := extractJSON(raw)
	if document == nil {
		return nil, apperror.Newf("OCR_UNPARSEABLE", "the model response contained no JSON object")
	}
	var body payload
	if err := json.Unmarshal(document, &body); err != nil {
		return nil, apperror.Newf("OCR_UNPARSEABLE", "the model response was not the expected shape")
	}

	result := &receiptdomain.ExtractionResult{
		Merchant: strings.TrimSpace(body.Merchant),
		Currency: normalizeCurrency(body.Currency),
		Raw:      json.RawMessage(append([]byte(nil), raw...)),
	}
	if parsed, ok := parseDate(body.Date); ok {
		result.Date = &parsed
	}
	result.Subtotal, _ = amount(body.Subtotal)
	result.Tax, _ = amount(body.Tax)
	result.ServiceCharge, _ = amount(body.ServiceCharge)
	result.Total, _ = amount(body.Total)

	itemsTotal := decimal.Zero
	for _, line := range body.LineItems {
		name := strings.TrimSpace(line.Name)
		if name == "" {
			continue
		}
		item := receiptdomain.ExtractedItem{Name: name}
		quantity, quantityOK := amount(line.Quantity)
		if !quantityOK || !quantity.IsPositive() {
			quantity = decimal.NewFromInt(1) // Rule 2 of the prompt: a missing quantity means one.
		}
		item.Quantity = quantity
		unit, unitOK := amount(line.UnitPrice)
		total, totalOK := amount(line.TotalPrice)
		// OCR-3: a malformed number zeroes that field and flags the line rather than failing
		// the receipt. A line with a readable unit price but no total is worth reconstructing.
		if !totalOK || total.IsZero() {
			if unitOK && unit.IsPositive() {
				total = unit.Mul(quantity)
			} else {
				item.Flagged = true
			}
		}
		if !unitOK {
			item.Flagged = true
		}
		item.UnitPrice, item.TotalPrice = unit, total
		itemsTotal = itemsTotal.Add(total)
		result.Items = append(result.Items, item)
	}

	extras := result.Tax.Add(result.ServiceCharge)
	// OCR-5: no total on the receipt means the total is what the parts add up to.
	if result.Total.IsZero() {
		result.Total = itemsTotal.Add(extras)
	}
	// OCR-4: the parts do not reconcile with the printed total. Still usable, but say so.
	if !money.IsZero(itemsTotal.Add(extras).Sub(result.Total)) {
		result.Discrepancy = true
	}
	if result.Subtotal.IsZero() {
		result.Subtotal = itemsTotal
	}
	return result, nil
}

// extractJSON finds the JSON object inside whatever the model wrapped it in: a bare object, a
// ```json fence, or a paragraph of explanation with the object somewhere in the middle.
func extractJSON(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if json.Valid(trimmed) && bytes.HasPrefix(trimmed, []byte("{")) {
		return trimmed
	}
	if fenced := betweenFences(trimmed); fenced != nil {
		return fenced
	}
	// Fall back to the widest brace-balanced span, which survives prose on both sides.
	start := bytes.IndexByte(trimmed, '{')
	if start < 0 {
		return nil
	}
	depth, inString, escaped := 0, false, false
	for index := start; index < len(trimmed); index++ {
		char := trimmed[index]
		switch {
		case escaped:
			escaped = false
		case char == '\\' && inString:
			escaped = true
		case char == '"':
			inString = !inString
		case inString:
			// Braces inside a string are text, not structure.
		case char == '{':
			depth++
		case char == '}':
			depth--
			if depth == 0 {
				candidate := trimmed[start : index+1]
				if json.Valid(candidate) {
					return candidate
				}
				return nil
			}
		}
	}
	return nil
}

func betweenFences(data []byte) []byte {
	start := bytes.Index(data, []byte("```"))
	if start < 0 {
		return nil
	}
	rest := data[start+3:]
	if newline := bytes.IndexByte(rest, '\n'); newline >= 0 {
		rest = rest[newline+1:]
	}
	end := bytes.Index(rest, []byte("```"))
	if end < 0 {
		return nil
	}
	candidate := bytes.TrimSpace(rest[:end])
	if json.Valid(candidate) && bytes.HasPrefix(candidate, []byte("{")) {
		return candidate
	}
	return nil
}

// amount reads a money field whether the model sent a number, a quoted number, a currency-prefixed
// string, or null. The bool reports whether anything usable was found.
func amount(raw json.RawMessage) (decimal.Decimal, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return decimal.Zero, false
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err != nil {
		// Not a string, so try it as a bare number.
		value, err := decimal.NewFromString(string(trimmed))
		if err != nil {
			return decimal.Zero, false
		}
		return value, true
	}
	value, err := decimal.NewFromString(cleanNumeric(text))
	if err != nil {
		return decimal.Zero, false
	}
	return value, true
}

// cleanNumeric strips the decoration a model leaves on a quoted amount — currency symbols, spaces,
// and thousands separators — without touching the decimal point.
func cleanNumeric(text string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(text) {
		switch {
		case char >= '0' && char <= '9', char == '.', char == '-':
			builder.WriteRune(char)
		case char == ',':
			// A comma is a thousands separator here; the prompt asks for a dot decimal.
		}
	}
	return builder.String()
}

func parseDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "01/02/2006", time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// normalizeCurrency keeps only codes this app can actually convert; anything else is dropped so the
// receipt falls back to the trip's own currency rather than carrying a made-up one (rule: ask for
// ISO-4217 and reject anything else).
func normalizeCurrency(value string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	if !money.IsSupportedCurrency(code) {
		return ""
	}
	return code
}
