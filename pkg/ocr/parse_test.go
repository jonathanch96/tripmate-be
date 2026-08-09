package ocr

import (
	"testing"

	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/shopspring/decimal"
)

const cleanJSON = `{
  "merchant": "Warung Bu Made",
  "date": "2026-08-12",
  "currency": "IDR",
  "lineItems": [
    {"name": "Nasi Goreng", "quantity": 2, "unitPrice": 45000, "totalPrice": 90000},
    {"name": "Es Teh", "quantity": 3, "unitPrice": 10000, "totalPrice": 30000}
  ],
  "subtotal": 120000,
  "tax": 12000,
  "serviceCharge": 6000,
  "total": 138000
}`

// Test-plan item 1: every way the model has been seen to answer.
func TestParseHandlesEveryResponseShape(t *testing.T) {
	for _, test := range []struct {
		name            string
		raw             string
		wantMerchant    string
		wantItems       int
		wantTotal       string
		wantCurrency    string
		wantFlagged     int
		wantDiscrepancy bool
	}{
		{name: "clean json", raw: cleanJSON, wantMerchant: "Warung Bu Made", wantItems: 2, wantTotal: "138000", wantCurrency: "IDR"},
		{name: "fenced json", raw: "```json\n" + cleanJSON + "\n```", wantMerchant: "Warung Bu Made", wantItems: 2, wantTotal: "138000", wantCurrency: "IDR"},
		{name: "bare fence without a language", raw: "```\n" + cleanJSON + "\n```", wantMerchant: "Warung Bu Made", wantItems: 2, wantTotal: "138000", wantCurrency: "IDR"},
		{name: "prose either side of the json", raw: "Sure! Here is the receipt data:\n" + cleanJSON + "\nLet me know if you need anything else.", wantMerchant: "Warung Bu Made", wantItems: 2, wantTotal: "138000", wantCurrency: "IDR"},
		{
			name: "amounts as strings with currency decoration",
			raw: `{"merchant":"Cafe","currency":"PHP","lineItems":[
				{"name":"Latte","quantity":"1","unitPrice":"₱180.00","totalPrice":"₱180.00"},
				{"name":"Cake","quantity":"2","unitPrice":"1,250.50","totalPrice":"2,501.00"}],
				"tax":"0","serviceCharge":"0","total":"2,681.00"}`,
			wantMerchant: "Cafe", wantItems: 2, wantTotal: "2681", wantCurrency: "PHP",
		},
		{
			name: "missing total falls back to the parts",
			raw: `{"merchant":"Bar","currency":"USD","lineItems":[
				{"name":"Beer","quantity":2,"unitPrice":5,"totalPrice":10}],"tax":1,"serviceCharge":2}`,
			wantMerchant: "Bar", wantItems: 1, wantTotal: "13", wantCurrency: "USD",
		},
		{
			name: "null total and null date are tolerated",
			raw: `{"merchant":"Bar","date":null,"currency":"USD","lineItems":[
				{"name":"Beer","quantity":1,"unitPrice":7,"totalPrice":7}],"tax":null,"serviceCharge":null,"total":null}`,
			wantMerchant: "Bar", wantItems: 1, wantTotal: "7", wantCurrency: "USD",
		},
		{
			name: "a malformed line is flagged, not fatal",
			raw: `{"merchant":"Diner","currency":"USD","lineItems":[
				{"name":"Soup","quantity":1,"unitPrice":"n/a","totalPrice":"n/a"},
				{"name":"Steak","quantity":1,"unitPrice":30,"totalPrice":30}],"total":30}`,
			wantMerchant: "Diner", wantItems: 2, wantTotal: "30", wantCurrency: "USD", wantFlagged: 1,
		},
		{
			name: "a line with a unit price but no total is reconstructed",
			raw: `{"merchant":"Deli","currency":"USD","lineItems":[
				{"name":"Bagel","quantity":3,"unitPrice":4,"totalPrice":null}],"total":12}`,
			wantMerchant: "Deli", wantItems: 1, wantTotal: "12", wantCurrency: "USD",
		},
		{
			name: "numbers that do not reconcile are reported",
			raw: `{"merchant":"Shop","currency":"USD","lineItems":[
				{"name":"Thing","quantity":1,"unitPrice":10,"totalPrice":10}],"tax":1,"serviceCharge":0,"total":99}`,
			wantMerchant: "Shop", wantItems: 1, wantTotal: "99", wantCurrency: "USD", wantDiscrepancy: true,
		},
		{
			name: "an unsupported currency code is dropped rather than invented",
			raw: `{"merchant":"Shop","currency":"XYZ","lineItems":[
				{"name":"Thing","quantity":1,"unitPrice":10,"totalPrice":10}],"total":10}`,
			wantMerchant: "Shop", wantItems: 1, wantTotal: "10", wantCurrency: "",
		},
		{
			name: "nameless lines are dropped",
			raw: `{"merchant":"Shop","currency":"USD","lineItems":[
				{"name":"","quantity":1,"unitPrice":10,"totalPrice":10},
				{"name":"Real","quantity":1,"unitPrice":5,"totalPrice":5}],"total":5}`,
			wantMerchant: "Shop", wantItems: 1, wantTotal: "5", wantCurrency: "USD",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := Parse([]byte(test.raw))
			if err != nil {
				t.Fatal(err)
			}
			if result.Merchant != test.wantMerchant {
				t.Errorf("merchant = %q, want %q", result.Merchant, test.wantMerchant)
			}
			if len(result.Items) != test.wantItems {
				t.Errorf("items = %d, want %d (%+v)", len(result.Items), test.wantItems, result.Items)
			}
			if !result.Total.Equal(decimal.RequireFromString(test.wantTotal)) {
				t.Errorf("total = %s, want %s", result.Total, test.wantTotal)
			}
			if result.Currency != test.wantCurrency {
				t.Errorf("currency = %q, want %q", result.Currency, test.wantCurrency)
			}
			flagged := 0
			for _, item := range result.Items {
				if item.Flagged {
					flagged++
				}
			}
			if flagged != test.wantFlagged {
				t.Errorf("flagged items = %d, want %d", flagged, test.wantFlagged)
			}
			if result.Discrepancy != test.wantDiscrepancy {
				t.Errorf("discrepancy = %v, want %v", result.Discrepancy, test.wantDiscrepancy)
			}
			// The raw response is always kept, whatever else happened (OCR-2 debugging).
			if len(result.Raw) == 0 {
				t.Error("the raw response was not retained")
			}
		})
	}
}

func TestParseRejectsResponsesWithNoJSON(t *testing.T) {
	for _, test := range []struct{ name, raw string }{
		{name: "prose only", raw: "I'm sorry, I can't read this receipt."},
		{name: "empty", raw: ""},
		{name: "truncated object", raw: `{"merchant":"Shop","lineItems":[`},
		{name: "json array rather than an object", raw: `["not", "an", "object"]`},
		{name: "unbalanced braces", raw: `{{{`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse([]byte(test.raw)); !apperror.Is(err, "OCR_UNPARSEABLE") {
				t.Fatalf("Parse(%s) error = %v, want OCR_UNPARSEABLE", test.name, err)
			}
		})
	}
}

func TestParseReadsTheDateWhenItIsThere(t *testing.T) {
	result, err := Parse([]byte(cleanJSON))
	if err != nil {
		t.Fatal(err)
	}
	if result.Date == nil {
		t.Fatal("date was not parsed")
	}
	if got := result.Date.Format("2006-01-02"); got != "2026-08-12" {
		t.Fatalf("date = %s, want 2026-08-12", got)
	}
	if !result.Subtotal.Equal(decimal.RequireFromString("120000")) {
		t.Fatalf("subtotal = %s, want 120000", result.Subtotal)
	}
	if !result.Tax.Equal(decimal.RequireFromString("12000")) || !result.ServiceCharge.Equal(decimal.RequireFromString("6000")) {
		t.Fatalf("tax/service = %s/%s", result.Tax, result.ServiceCharge)
	}
}

// A brace inside a string value must not end the object early.
func TestParseSurvivesBracesInsideStrings(t *testing.T) {
	raw := `Here you go: {"merchant":"Curly } Braces {Cafe}","currency":"USD","lineItems":[{"name":"Tea","quantity":1,"unitPrice":3,"totalPrice":3}],"total":3} done`
	result, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if result.Merchant != "Curly } Braces {Cafe}" {
		t.Fatalf("merchant = %q", result.Merchant)
	}
}
