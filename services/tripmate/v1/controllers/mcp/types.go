package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/shopspring/decimal"
)

// Money is a decimal amount. AI tools send amounts as JSON numbers or strings interchangeably, so
// both are accepted and parsed exactly - never through float64.
type Money string

func (m *Money) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, `"`) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		text = value
	}
	if text == "null" {
		text = ""
	}
	*m = Money(strings.TrimSpace(text))
	return nil
}

func (m Money) IsZero() bool { return strings.TrimSpace(string(m)) == "" }

func (m Money) Decimal(field string) (decimal.Decimal, error) {
	if m.IsZero() {
		return decimal.Zero, nil
	}
	value, err := decimal.NewFromString(string(m))
	if err != nil {
		return decimal.Zero, fmt.Errorf("%s must be a plain number without thousands separators, like 45000 or 12.50 - got %q", field, string(m))
	}
	return value, nil
}

// thousandsGrouped matches 45.000 or 1.250.000 - how many receipts print whole amounts.
var thousandsGrouped = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)

// DecimalIn parses the amount for a currency, refusing a dot-grouped whole number in a currency
// without minor units (IDR 45.000 means forty-five thousand, never forty-five).
func (m Money) DecimalIn(field, currency string) (decimal.Decimal, error) {
	if money.DisplayScale(currency) == 0 && thousandsGrouped.MatchString(strings.TrimSpace(string(m))) {
		return decimal.Zero, fmt.Errorf("%s is %q: %s has no decimals, so write it without separators (e.g. %s)",
			field, string(m), currency, strings.ReplaceAll(string(m), ".", ""))
	}
	return m.Decimal(field)
}

// schemaOptions teaches the schema generator that Money may be a string or a number.
var schemaOptions = &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[Money](): {Types: []string{"string", "number"}},
}}

func inputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](schemaOptions)
	if err != nil {
		panic(err)
	}
	return schema
}

// ---- inputs

type tripInput struct {
	TripCode string `json:"trip_code" jsonschema:"the trip's code, as returned by list_trips or get_active_trip"`
}

type listTripsInput struct {
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"also include archived trips"`
}

type activeTripInput struct {
	Date string `json:"date" jsonschema:"YYYY-MM-DD: the date printed on the bill, or the user's local date today if there is none"`
}

type listExpensesInput struct {
	TripCode string `json:"trip_code" jsonschema:"the trip's code"`
	DateFrom string `json:"date_from,omitempty" jsonschema:"YYYY-MM-DD, inclusive"`
	DateTo   string `json:"date_to,omitempty" jsonschema:"YYYY-MM-DD, inclusive"`
	Query    string `json:"query,omitempty" jsonschema:"text to search in descriptions"`
	Page     int    `json:"page,omitempty" jsonschema:"page number, starting at 1 (25 per page)"`
}

type payerInput struct {
	UserID string `json:"user_id" jsonschema:"participant user_id"`
	Amount Money  `json:"amount" jsonschema:"how much this person paid"`
}

type splitInput struct {
	UserID string `json:"user_id" jsonschema:"participant user_id"`
	Value  Money  `json:"value" jsonschema:"amount owed (manual), percentage 0-100 (percent), or share count (shares)"`
}

type createTripInput struct {
	Name         string `json:"name" jsonschema:"trip name, e.g. Bali 2026"`
	BaseCurrency string `json:"base_currency" jsonschema:"ISO 4217 code balances are settled in, e.g. IDR"`
	StartDate    string `json:"start_date" jsonschema:"YYYY-MM-DD"`
	EndDate      string `json:"end_date" jsonschema:"YYYY-MM-DD"`
	Country      string `json:"country,omitempty" jsonschema:"optional destination country"`
}

type addExpenseInput struct {
	TripCode     string       `json:"trip_code" jsonschema:"the trip's code"`
	Description  string       `json:"description" jsonschema:"what it was for, e.g. Taxi to airport"`
	Date         string       `json:"date" jsonschema:"YYYY-MM-DD"`
	Amount       Money        `json:"amount" jsonschema:"total amount"`
	Currency     string       `json:"currency" jsonschema:"ISO 4217 code, usually the trip's base currency"`
	SplitType    string       `json:"split_type" jsonschema:"equal | manual | percent | shares"`
	PaidBy       []payerInput `json:"paid_by" jsonschema:"who paid; amounts must add up to the total"`
	Participants []string     `json:"participants,omitempty" jsonschema:"equal split only: user_ids sharing the cost equally"`
	Splits       []splitInput `json:"splits,omitempty" jsonschema:"manual / percent / shares splits: one entry per person"`
	CategoryID   string       `json:"category_id,omitempty" jsonschema:"optional category id from get_trip"`
	Note         string       `json:"note,omitempty"`
}

type billItemInput struct {
	Name    string   `json:"name" jsonschema:"item name as printed"`
	Amount  Money    `json:"amount" jsonschema:"line total for this item (quantity x price, after any item-level discount)"`
	UserIDs []string `json:"user_ids" jsonschema:"user_ids of everyone who shared this item (at least one)"`
}

type billExpenseInput struct {
	TripCode      string          `json:"trip_code" jsonschema:"the trip's code"`
	Description   string          `json:"description" jsonschema:"usually the merchant name, e.g. Warung Made"`
	Date          string          `json:"date" jsonschema:"YYYY-MM-DD from the bill"`
	Currency      string          `json:"currency" jsonschema:"ISO 4217 code of the bill"`
	Items         []billItemInput `json:"items" jsonschema:"every line on the bill with who shared it"`
	Tax           Money           `json:"tax,omitempty" jsonschema:"bill-level tax amount, if any"`
	ServiceCharge Money           `json:"service_charge,omitempty" jsonschema:"bill-level service charge amount, if any"`
	Discount      Money           `json:"discount,omitempty" jsonschema:"bill-level discount amount as a positive number, if any"`
	Total         Money           `json:"total,omitempty" jsonschema:"grand total as printed; used to check the numbers add up"`
	PaidBy        []payerInput    `json:"paid_by" jsonschema:"who paid; amounts must add up to the total"`
	CategoryID    string          `json:"category_id,omitempty" jsonschema:"optional category id from get_trip"`
	Preview       bool            `json:"preview,omitempty" jsonschema:"true to calculate and return each person's share without saving"`
}

type settlementInput struct {
	TripCode   string `json:"trip_code" jsonschema:"the trip's code"`
	FromUserID string `json:"from_user_id" jsonschema:"user_id of the person who paid the money back"`
	ToUserID   string `json:"to_user_id" jsonschema:"user_id of the person who received it"`
	Amount     Money  `json:"amount" jsonschema:"amount repaid"`
	Currency   string `json:"currency,omitempty" jsonschema:"ISO 4217 code; defaults to the trip's base currency"`
	Method     string `json:"method,omitempty" jsonschema:"cash | bank_transfer (default cash)"`
	Date       string `json:"date,omitempty" jsonschema:"YYYY-MM-DD; defaults to today"`
	Note       string `json:"note,omitempty"`
}

type inviteInput struct {
	TripCode string `json:"trip_code" jsonschema:"the trip's code"`
	Email    string `json:"email" jsonschema:"email address of the person to add"`
}

// ---- outputs

type participantOut struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

type categoryOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type tripOut struct {
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	BaseCurrency string           `json:"base_currency"`
	Country      string           `json:"country,omitempty"`
	StartDate    string           `json:"start_date"`
	EndDate      string           `json:"end_date"`
	Status       string           `json:"status"`
	YourRole     string           `json:"your_role,omitempty"`
	Participants []participantOut `json:"participants,omitempty"`
	Categories   []categoryOut    `json:"categories,omitempty"`
}

type tripsOut struct {
	Trips []tripOut `json:"trips"`
}

type activeTripOut struct {
	Match   string    `json:"match" jsonschema:"one | multiple | none"`
	Message string    `json:"message"`
	Trips   []tripOut `json:"trips"`
}

type amountByUser struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

type expenseOut struct {
	ID          string         `json:"id"`
	Date        string         `json:"date"`
	Description string         `json:"description"`
	Amount      string         `json:"amount"`
	Currency    string         `json:"currency"`
	Status      string         `json:"status"`
	SplitType   string         `json:"split_type"`
	PaidBy      []amountByUser `json:"paid_by"`
	Owed        []amountByUser `json:"owed"`
	Note        string         `json:"note,omitempty"`
	CreatedVia  string         `json:"created_via,omitempty"`
}

type expensesOut struct {
	Expenses   []expenseOut `json:"expenses"`
	Page       int          `json:"page"`
	TotalCount int64        `json:"total_count"`
}

type savedExpenseOut struct {
	Expense   expenseOut `json:"expense"`
	Duplicate bool       `json:"duplicate,omitempty" jsonschema:"true when an identical expense had just been saved, so nothing new was created"`
	Message   string     `json:"message"`
}

type billShareOut struct {
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	ItemsSubtotal string `json:"items_subtotal"`
	Extras        string `json:"tax_service_discount" jsonschema:"this person's share of tax + service charge - discount"`
	Total         string `json:"total"`
}

type billOut struct {
	Preview    bool           `json:"preview"`
	Currency   string         `json:"currency"`
	ItemsTotal string         `json:"items_total"`
	Extras     string         `json:"tax_service_discount"`
	Total      string         `json:"total"`
	Shares     []billShareOut `json:"shares"`
	Expense    *expenseOut    `json:"expense,omitempty"`
	Duplicate  bool           `json:"duplicate,omitempty"`
	Message    string         `json:"message"`
}

type balanceOut struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Paid   string `json:"paid"`
	Owed   string `json:"owed"`
	Net    string `json:"net" jsonschema:"positive: is owed money; negative: owes money"`
}

type transferOut struct {
	FromUserID string `json:"from_user_id"`
	From       string `json:"from"`
	ToUserID   string `json:"to_user_id"`
	To         string `json:"to"`
	Amount     string `json:"amount"`
	Currency   string `json:"currency"`
}

type balancesOut struct {
	BaseCurrency string        `json:"base_currency"`
	Balances     []balanceOut  `json:"balances"`
	Transfers    []transferOut `json:"suggested_transfers" jsonschema:"the fewest payments that settle everyone up"`
}

type settlementOut struct {
	ID       string `json:"id"`
	Date     string `json:"date"`
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Method   string `json:"method"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
}

type settlementsOut struct {
	Settlements []settlementOut `json:"settlements"`
}

type savedSettlementOut struct {
	Settlement settlementOut `json:"settlement"`
	Message    string        `json:"message"`
}

type inviteOut struct {
	Status  string `json:"status" jsonschema:"added (already had an account) | invited (new account placeholder created)"`
	UserID  string `json:"user_id,omitempty"`
	Message string `json:"message"`
}
