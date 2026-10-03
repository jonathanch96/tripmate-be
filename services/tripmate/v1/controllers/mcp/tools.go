package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"
)

const (
	dateLayout = "2006-01-02"
	// duplicateWindow is how recently an identical expense must have been saved by the same user
	// for a repeat call to be treated as a retry rather than a second purchase.
	duplicateWindow = 2 * time.Minute
	expensesPerPage = 25
)

func readOnly(title string) *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &closed}
}

func writes(title string) *mcp.ToolAnnotations {
	closed, destructive := false, false
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &destructive, OpenWorldHint: &closed}
}

func (c *controller) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "list_trips", Annotations: readOnly("List trips"), InputSchema: inputSchema[listTripsInput](),
		Description: "List the user's TripMate trips (newest first) with their codes, dates and status."}, c.listTrips)
	mcp.AddTool(server, &mcp.Tool{Name: "get_active_trip", Annotations: readOnly("Find the active trip"), InputSchema: inputSchema[activeTripInput](),
		Description: "Find the trip a bill or expense belongs to: trips that are open (not archived or finalized) and whose dates include the given date, with their participants. " +
			"Start here when splitting a bill. If match is \"multiple\", ask the user which trip - never guess. If match is \"none\", show the listed recent trips and ask."}, c.getActiveTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "get_trip", Annotations: readOnly("Get trip details"), InputSchema: inputSchema[tripInput](),
		Description: "Get one trip's details: dates, base currency, whether other currencies are allowed and the exchange rates it has, status, participants (with user_id) and expense categories."}, c.getTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "list_expenses", Annotations: readOnly("List expenses"), InputSchema: inputSchema[listExpensesInput](),
		Description: "List a trip's expenses, newest first, with who paid and who owes what. 25 per page."}, c.listExpenses)
	mcp.AddTool(server, &mcp.Tool{Name: "get_balances", Annotations: readOnly("Get balances"), InputSchema: inputSchema[tripInput](),
		Description: "Get each participant's balance on a trip (in the base currency) and the suggested payments that would settle everyone up."}, c.getBalances)
	mcp.AddTool(server, &mcp.Tool{Name: "list_settlements", Annotations: readOnly("List settlements"), InputSchema: inputSchema[tripInput](),
		Description: "List repayments recorded between participants on a trip."}, c.listSettlements)

	mcp.AddTool(server, &mcp.Tool{Name: "create_bill_expense", Annotations: writes("Split a bill"), InputSchema: inputSchema[billExpenseInput](),
		Description: "Save an itemised bill as one expense, split by who had what. You read the bill; pass every item with its line amount and the user_ids who shared it, " +
			"plus bill-level tax, service charge and discount separately - TripMate spreads those in proportion to what each person had. " +
			"Call with preview=true first and show the user each person's total; save with preview=false after they confirm. Amounts are plain numbers as printed."}, c.createBillExpense)
	mcp.AddTool(server, &mcp.Tool{Name: "add_expense", Annotations: writes("Add an expense"), InputSchema: inputSchema[addExpenseInput](),
		Description: "Add a non-itemised expense to a trip (taxi, hotel, tickets...). split_type equal needs participants; manual, percent and shares need splits. " +
			"For a restaurant bill or receipt with line items, use create_bill_expense instead."}, c.addExpense)
	mcp.AddTool(server, &mcp.Tool{Name: "record_settlement", Annotations: writes("Record a repayment"), InputSchema: inputSchema[settlementInput](),
		Description: "Record that one participant paid another back. Use get_balances to see who owes whom."}, c.recordSettlement)
	mcp.AddTool(server, &mcp.Tool{Name: "set_exchange_rate", Annotations: writes("Set an exchange rate"), InputSchema: inputSchema[setRateInput](),
		Description: "Save how much one unit of a foreign currency is worth in the trip's base currency (trip planners only). Use it when an expense or repayment fails because the trip has no rate for its currency: " +
			"ask the user for the rate first and pass the number they confirm. It replaces any earlier rate for that currency and applies to every amount in that currency on the trip."}, c.setExchangeRate)
	mcp.AddTool(server, &mcp.Tool{Name: "create_trip", Annotations: writes("Create a trip"), InputSchema: inputSchema[createTripInput](),
		Description: "Create a new trip with the user as its planner. Confirm the name, dates and base currency with the user first."}, c.createTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "invite_participant", Annotations: writes("Add a participant"), InputSchema: inputSchema[inviteInput](),
		Description: "Add someone to a trip by email (trip planners only). If they have no TripMate account yet, they are added right away and the trip appears when they sign up with that email."}, c.invite)
}

// ---- read tools

func (c *controller) listTrips(ctx context.Context, req *mcp.CallToolRequest, in listTripsInput) (*mcp.CallToolResult, tripsOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, tripsOut{}, err
	}
	filter := tripdomain.ListFilter{Page: 1, PerPage: 50}
	if !in.IncludeArchived {
		active := false
		filter.Archived = &active
	}
	rows, _, err := c.deps.Trips.ListMine(ctx, k.who.UserID, filter)
	if err != nil {
		return nil, tripsOut{}, toolError(err)
	}
	out := tripsOut{Trips: make([]tripOut, 0, len(rows))}
	for _, trip := range rows {
		out.Trips = append(out.Trips, tripSummary(trip))
	}
	return nil, out, nil
}

func (c *controller) getActiveTrip(ctx context.Context, req *mcp.CallToolRequest, in activeTripInput) (*mcp.CallToolResult, activeTripOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, activeTripOut{}, err
	}
	day, err := parseDate("date", in.Date)
	if err != nil {
		return nil, activeTripOut{}, err
	}
	active := false
	rows, _, err := c.deps.Trips.ListMine(ctx, k.who.UserID, tripdomain.ListFilter{Page: 1, PerPage: 100, Archived: &active})
	if err != nil {
		return nil, activeTripOut{}, toolError(err)
	}
	matches := make([]tripOut, 0)
	for _, trip := range rows {
		if trip.IsFinalized || trip.IsArchived || day.Before(dateOnly(trip.StartDate)) || day.After(dateOnly(trip.EndDate)) {
			continue
		}
		detail, err := c.tripDetail(ctx, k, trip.Code)
		if err != nil {
			return nil, activeTripOut{}, toolError(err)
		}
		matches = append(matches, *detail)
	}
	switch len(matches) {
	case 1:
		return nil, activeTripOut{Match: "one", Trips: matches,
			Message: fmt.Sprintf("Use trip %s (%s).", matches[0].Name, matches[0].Code)}, nil
	case 0:
		recent := make([]tripOut, 0, 5)
		for _, trip := range rows {
			if len(recent) == 5 {
				break
			}
			recent = append(recent, tripSummary(trip))
		}
		return nil, activeTripOut{Match: "none", Trips: recent,
			Message: "No open trip covers " + in.Date + ". Show the user these recent trips and ask which one to use, or offer to create a trip."}, nil
	default:
		return nil, activeTripOut{Match: "multiple", Trips: matches,
			Message: "More than one open trip covers " + in.Date + ". Ask the user which trip this is for before continuing."}, nil
	}
}

func (c *controller) getTrip(ctx context.Context, req *mcp.CallToolRequest, in tripInput) (*mcp.CallToolResult, tripOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, tripOut{}, err
	}
	detail, err := c.tripDetail(ctx, k, in.TripCode)
	if err != nil {
		return nil, tripOut{}, toolError(err)
	}
	return nil, *detail, nil
}

func (c *controller) listExpenses(ctx context.Context, req *mcp.CallToolRequest, in listExpensesInput) (*mcp.CallToolResult, expensesOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, expensesOut{}, err
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, expensesOut{}, toolError(err)
	}
	page := max(in.Page, 1)
	filter := expensedomain.Filter{Page: page, PerPage: expensesPerPage, Query: strings.TrimSpace(in.Query)}
	if in.DateFrom != "" {
		from, err := parseDate("date_from", in.DateFrom)
		if err != nil {
			return nil, expensesOut{}, err
		}
		filter.DateFrom = &from
	}
	if in.DateTo != "" {
		to, err := parseDate("date_to", in.DateTo)
		if err != nil {
			return nil, expensesOut{}, err
		}
		filter.DateTo = &to
	}
	rows, total, _, err := c.deps.Expenses.List(ctx, *tc, filter)
	if err != nil {
		return nil, expensesOut{}, toolError(err)
	}
	names, err := c.names(ctx, k, tc)
	if err != nil {
		return nil, expensesOut{}, toolError(err)
	}
	out := expensesOut{Expenses: make([]expenseOut, 0, len(rows)), Page: page, TotalCount: total}
	for _, row := range rows {
		out.Expenses = append(out.Expenses, expenseSummary(row, names))
	}
	return nil, out, nil
}

func (c *controller) getBalances(ctx context.Context, req *mcp.CallToolRequest, in tripInput) (*mcp.CallToolResult, balancesOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, balancesOut{}, err
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, balancesOut{}, toolError(err)
	}
	result, err := c.deps.Balances.Calculate(ctx, *tc)
	if err != nil {
		return nil, balancesOut{}, toolError(err)
	}
	names, err := c.names(ctx, k, tc)
	if err != nil {
		return nil, balancesOut{}, toolError(err)
	}
	scale := money.DisplayScale(result.BaseCurrency)
	out := balancesOut{BaseCurrency: result.BaseCurrency, Balances: make([]balanceOut, 0, len(result.Balances)), Transfers: make([]transferOut, 0, len(result.Debts))}
	for _, balance := range result.Balances {
		out.Balances = append(out.Balances, balanceOut{UserID: balance.UserID.String(), Name: names.of(balance.UserID),
			Paid: balance.TotalPaid.StringFixedBank(scale), Owed: balance.TotalOwed.StringFixedBank(scale), Net: balance.NetBalance.StringFixedBank(scale)})
	}
	for _, debt := range result.Debts {
		out.Transfers = append(out.Transfers, transferOut{FromUserID: debt.FromUserID.String(), From: names.of(debt.FromUserID),
			ToUserID: debt.ToUserID.String(), To: names.of(debt.ToUserID), Amount: debt.Amount.StringFixedBank(money.DisplayScale(debt.Currency)), Currency: debt.Currency})
	}
	return nil, out, nil
}

func (c *controller) listSettlements(ctx context.Context, req *mcp.CallToolRequest, in tripInput) (*mcp.CallToolResult, settlementsOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, settlementsOut{}, err
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, settlementsOut{}, toolError(err)
	}
	rows, _, err := c.deps.Settlements.List(ctx, *tc, settlementdomain.Filter{Page: 1, PerPage: 100})
	if err != nil {
		return nil, settlementsOut{}, toolError(err)
	}
	names, err := c.names(ctx, k, tc)
	if err != nil {
		return nil, settlementsOut{}, toolError(err)
	}
	out := settlementsOut{Settlements: make([]settlementOut, 0, len(rows))}
	for _, row := range rows {
		out.Settlements = append(out.Settlements, settlementSummary(row, names))
	}
	return nil, out, nil
}

// ---- write tools

func (c *controller) createBillExpense(ctx context.Context, req *mcp.CallToolRequest, in billExpenseInput) (*mcp.CallToolResult, billOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, billOut{}, err
	}
	if !in.Preview {
		if err := k.requireWrite(); err != nil {
			return nil, billOut{}, err
		}
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, billOut{}, toolError(err)
	}
	names, err := c.names(ctx, k, tc)
	if err != nil {
		return nil, billOut{}, toolError(err)
	}
	conv, err := c.currencyFor(ctx, k, tc, in.Currency)
	if err != nil {
		return nil, billOut{}, err
	}
	currency := conv.Currency
	day, err := parseDate("date", in.Date)
	if err != nil {
		return nil, billOut{}, err
	}
	if len(in.Items) == 0 {
		return nil, billOut{}, errors.New("items is required: list every line on the bill with who shared it")
	}
	items := make([]expensedomain.ItemAssignment, 0, len(in.Items))
	itemsTotal := decimal.Zero
	for index, item := range in.Items {
		label := fmt.Sprintf("items[%d] (%s)", index, item.Name)
		amount, err := item.Amount.DecimalIn(label+".amount", currency)
		if err != nil {
			return nil, billOut{}, err
		}
		if amount.IsNegative() {
			return nil, billOut{}, fmt.Errorf("%s has a negative amount; apply discounts to the item's amount or pass a bill-level discount", label)
		}
		if len(item.UserIDs) == 0 {
			return nil, billOut{}, fmt.Errorf("%s is not assigned to anyone; ask the user who had it", label)
		}
		userIDs, err := names.resolve(label+".user_ids", item.UserIDs)
		if err != nil {
			return nil, billOut{}, err
		}
		items = append(items, expensedomain.ItemAssignment{Amount: amount, UserIDs: userIDs})
		itemsTotal = itemsTotal.Add(amount)
	}
	tax, err := in.Tax.DecimalIn("tax", currency)
	if err != nil {
		return nil, billOut{}, err
	}
	service, err := in.ServiceCharge.DecimalIn("service_charge", currency)
	if err != nil {
		return nil, billOut{}, err
	}
	discount, err := in.Discount.DecimalIn("discount", currency)
	if err != nil {
		return nil, billOut{}, err
	}
	if tax.IsNegative() || service.IsNegative() || discount.IsNegative() {
		return nil, billOut{}, errors.New("tax, service_charge and discount must be zero or positive (pass the discount as a positive amount)")
	}
	extras := tax.Add(service).Sub(discount)
	computed := itemsTotal.Add(extras)
	scale := money.DisplayScale(currency)
	total := computed
	if !in.Total.IsZero() {
		if total, err = in.Total.DecimalIn("total", currency); err != nil {
			return nil, billOut{}, err
		}
		if !total.RoundBank(scale).Equal(computed.RoundBank(scale)) {
			return nil, billOut{}, fmt.Errorf("the numbers do not add up: items %s + tax %s + service %s - discount %s = %s, but total is %s. "+
				"Re-check the bill (a missed item, a misread amount, or tax/service already included in item prices) and ask the user if unsure",
				itemsTotal.StringFixedBank(scale), tax.StringFixedBank(scale), service.StringFixedBank(scale), discount.StringFixedBank(scale),
				computed.StringFixedBank(scale), total.StringFixedBank(scale))
		}
	}
	if !total.IsPositive() {
		return nil, billOut{}, errors.New("the bill total must be more than zero")
	}
	payers, err := c.payers(in.PaidBy, names, currency)
	if err != nil {
		return nil, billOut{}, err
	}
	if err := expensedomain.ValidatePayers(total, currency, payers); err != nil {
		return nil, billOut{}, fmt.Errorf("paid_by must add up to the total %s %s: %w", total.StringFixedBank(scale), currency, toolError(err))
	}
	subtotals, err := expensedomain.CalculateSplits(expensedomain.SplitInput{Currency: currency, SplitType: domainexpense.SplitItem, Items: items})
	if err != nil {
		return nil, billOut{}, toolError(err)
	}
	splits, err := expensedomain.CalculateSplits(expensedomain.SplitInput{Amount: total, Currency: currency, SplitType: domainexpense.SplitItem, Items: items, Extras: extras})
	if err != nil {
		return nil, billOut{}, toolError(err)
	}
	if err := expensedomain.ValidateExpenseParticipants(names.ids(), payers, splits); err != nil {
		return nil, billOut{}, toolError(err)
	}
	out := billOut{Preview: in.Preview, Currency: currency, ItemsTotal: itemsTotal.StringFixedBank(scale),
		Extras: extras.StringFixedBank(scale), Total: total.StringFixedBank(scale), InBase: conv.out(total), Shares: billShares(subtotals, splits, names, scale)}
	if in.Preview {
		out.Message = "Preview only - nothing saved. Show the user each person's total and save with preview=false once they confirm."
		return nil, out, nil
	}
	description := strings.TrimSpace(in.Description)
	if duplicate := c.recentDuplicate(ctx, k, tc, day, description, total, currency); duplicate != nil {
		summary := expenseSummary(*duplicate, names)
		out.Expense, out.Duplicate = &summary, true
		out.Message = "This bill was already saved a moment ago; nothing new was created."
		return nil, out, nil
	}
	categoryID, err := optionalUUID("category_id", in.CategoryID)
	if err != nil {
		return nil, billOut{}, err
	}
	note := billNote(in, names, scale)
	created, err := c.deps.Expenses.Create(ctx, k.who, *tc, expensedomain.CreateInput{
		ExpenseDate: day, Description: description, Amount: total, Currency: currency, CategoryID: categoryID,
		SplitType: domainexpense.SplitItem, Payers: payers, Items: items, Extras: extras, Note: &note,
		Source: domainexpense.SourceAssistant, CreatedVia: k.createdVia(),
	})
	if err != nil {
		return nil, billOut{}, toolError(err)
	}
	summary := expenseSummary(*created, names)
	out.Expense = &summary
	out.Message = savedMessage(created)
	return nil, out, nil
}

func (c *controller) addExpense(ctx context.Context, req *mcp.CallToolRequest, in addExpenseInput) (*mcp.CallToolResult, savedExpenseOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, savedExpenseOut{}, err
	}
	if err := k.requireWrite(); err != nil {
		return nil, savedExpenseOut{}, err
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, savedExpenseOut{}, toolError(err)
	}
	names, err := c.names(ctx, k, tc)
	if err != nil {
		return nil, savedExpenseOut{}, toolError(err)
	}
	day, err := parseDate("date", in.Date)
	if err != nil {
		return nil, savedExpenseOut{}, err
	}
	conv, err := c.currencyFor(ctx, k, tc, in.Currency)
	if err != nil {
		return nil, savedExpenseOut{}, err
	}
	currency := conv.Currency
	amount, err := in.Amount.DecimalIn("amount", currency)
	if err != nil {
		return nil, savedExpenseOut{}, err
	}
	payers, err := c.payers(in.PaidBy, names, currency)
	if err != nil {
		return nil, savedExpenseOut{}, err
	}
	input := expensedomain.CreateInput{ExpenseDate: day, Description: strings.TrimSpace(in.Description), Amount: amount, Currency: currency,
		SplitType: domainexpense.SplitType(strings.ToLower(strings.TrimSpace(in.SplitType))), Payers: payers,
		Source: domainexpense.SourceAssistant, CreatedVia: k.createdVia()}
	if input.CategoryID, err = optionalUUID("category_id", in.CategoryID); err != nil {
		return nil, savedExpenseOut{}, err
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		input.Note = &note
	}
	switch input.SplitType {
	case domainexpense.SplitEqual:
		if len(in.Participants) == 0 {
			return nil, savedExpenseOut{}, errors.New("an equal split needs participants: the user_ids sharing the cost")
		}
		if input.Participants, err = names.resolve("participants", in.Participants); err != nil {
			return nil, savedExpenseOut{}, err
		}
	case domainexpense.SplitManual, domainexpense.SplitPercent, domainexpense.SplitShares:
		if len(in.Splits) == 0 {
			return nil, savedExpenseOut{}, fmt.Errorf("a %s split needs splits: one entry per person", input.SplitType)
		}
		values := make(map[uuid.UUID]decimal.Decimal, len(in.Splits))
		for index, split := range in.Splits {
			ids, err := names.resolve(fmt.Sprintf("splits[%d].user_id", index), []string{split.UserID})
			if err != nil {
				return nil, savedExpenseOut{}, err
			}
			value, err := split.Value.DecimalIn(fmt.Sprintf("splits[%d].value", index), currency)
			if err != nil {
				return nil, savedExpenseOut{}, err
			}
			values[ids[0]] = value
		}
		if input.SplitType == domainexpense.SplitManual {
			input.Manual = values
		} else {
			input.Weights = values
		}
	default:
		return nil, savedExpenseOut{}, errors.New("split_type must be equal, manual, percent or shares (use create_bill_expense for itemised bills)")
	}
	if duplicate := c.recentDuplicate(ctx, k, tc, day, input.Description, amount, currency); duplicate != nil {
		return nil, savedExpenseOut{Expense: expenseSummary(*duplicate, names), InBase: conv.out(amount), Duplicate: true,
			Message: "This expense was already saved a moment ago; nothing new was created."}, nil
	}
	created, err := c.deps.Expenses.Create(ctx, k.who, *tc, input)
	if err != nil {
		return nil, savedExpenseOut{}, toolError(err)
	}
	return nil, savedExpenseOut{Expense: expenseSummary(*created, names), InBase: conv.out(amount), Message: savedMessage(created)}, nil
}

func (c *controller) recordSettlement(ctx context.Context, req *mcp.CallToolRequest, in settlementInput) (*mcp.CallToolResult, savedSettlementOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, savedSettlementOut{}, err
	}
	if err := k.requireWrite(); err != nil {
		return nil, savedSettlementOut{}, err
	}
	tc, err := c.trip(ctx, k, in.TripCode)
	if err != nil {
		return nil, savedSettlementOut{}, toolError(err)
	}
	names, err := c.names(ctx, k, tc)
	if err != nil {
		return nil, savedSettlementOut{}, toolError(err)
	}
	ids, err := names.resolve("from_user_id/to_user_id", []string{in.FromUserID, in.ToUserID})
	if err != nil {
		return nil, savedSettlementOut{}, err
	}
	conv, err := c.currencyFor(ctx, k, tc, in.Currency)
	if err != nil {
		return nil, savedSettlementOut{}, err
	}
	currency := conv.Currency
	amount, err := in.Amount.DecimalIn("amount", currency)
	if err != nil {
		return nil, savedSettlementOut{}, err
	}
	method := domainsettlement.Method(strings.ToLower(strings.TrimSpace(in.Method)))
	if method == "" {
		method = domainsettlement.MethodCash
	}
	if method != domainsettlement.MethodCash && method != domainsettlement.MethodBankTransfer {
		return nil, savedSettlementOut{}, errors.New("method must be cash or bank_transfer")
	}
	day := dateOnly(time.Now())
	if in.Date != "" {
		if day, err = parseDate("date", in.Date); err != nil {
			return nil, savedSettlementOut{}, err
		}
	}
	input := settlementdomain.RecordInput{FromUserID: ids[0], ToUserID: ids[1], Amount: amount, Currency: currency, Method: method, Date: day}
	if note := strings.TrimSpace(in.Note); note != "" {
		input.Note = &note
	}
	created, err := c.deps.Settlements.Record(ctx, k.who, *tc, input)
	if err != nil {
		return nil, savedSettlementOut{}, toolError(err)
	}
	message := "Repayment recorded."
	if created.Status == domainsettlement.StatusPending {
		message = "Repayment recorded; it waits for the trip planner's approval before it counts."
	}
	return nil, savedSettlementOut{Settlement: settlementSummary(*created, names), InBase: conv.out(amount), Message: message}, nil
}

func (c *controller) createTrip(ctx context.Context, req *mcp.CallToolRequest, in createTripInput) (*mcp.CallToolResult, tripOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, tripOut{}, err
	}
	if err := k.requireWrite(); err != nil {
		return nil, tripOut{}, err
	}
	start, err := parseDate("start_date", in.StartDate)
	if err != nil {
		return nil, tripOut{}, err
	}
	end, err := parseDate("end_date", in.EndDate)
	if err != nil {
		return nil, tripOut{}, err
	}
	input := tripdomain.CreateInput{Name: strings.TrimSpace(in.Name), BaseCurrency: strings.ToUpper(strings.TrimSpace(in.BaseCurrency)),
		StartDate: start, EndDate: end, Settings: domaintrip.Settings{EditPermission: domaintrip.EditEveryone,
			MultiCurrencyEnabled: true, AllowSettlementBeforeEnd: true}}
	if country := strings.TrimSpace(in.Country); country != "" {
		input.Country = &country
	}
	created, err := c.deps.Trips.Create(ctx, k.who.UserID, input)
	if err != nil {
		return nil, tripOut{}, toolError(err)
	}
	detail, err := c.tripDetail(ctx, k, created.Code)
	if err != nil {
		return nil, tripOut{}, toolError(err)
	}
	return nil, *detail, nil
}

func (c *controller) invite(ctx context.Context, req *mcp.CallToolRequest, in inviteInput) (*mcp.CallToolResult, inviteOut, error) {
	ctx, k, err := c.caller(ctx, req)
	if err != nil {
		return nil, inviteOut{}, err
	}
	if err := k.requireWrite(); err != nil {
		return nil, inviteOut{}, err
	}
	// An empty password asks for a password-less placeholder account: the person is on the trip
	// immediately and claims the account when they sign up or sign in with Google.
	result, err := c.deps.Invitations.Invite(ctx, k.who.UserID, strings.TrimSpace(in.TripCode), in.Email, "")
	if err != nil {
		return nil, inviteOut{}, toolError(err)
	}
	out := inviteOut{Status: result.Status}
	if result.Participant != nil {
		out.UserID = result.Participant.UserID.String()
	}
	if result.Status == "added" {
		out.Message = "They already had a TripMate account and are now on the trip."
	} else {
		out.Message = "They are on the trip now and can be included in expenses. The trip appears for them when they sign up or sign in with Google using this email."
	}
	return nil, out, nil
}

// ---- helpers

func (c *controller) tripDetail(ctx context.Context, k *caller, code string) (*tripOut, error) {
	tc, err := c.trip(ctx, k, code)
	if err != nil {
		return nil, err
	}
	parts, err := c.deps.Participants.List(ctx, k.who.UserID, tc.Trip.Code)
	if err != nil {
		return nil, err
	}
	categories, err := c.deps.Categories.List(ctx, tc.Trip.ID)
	if err != nil {
		return nil, err
	}
	out := tripSummary(tc.Trip)
	out.YourRole = string(tc.Participant.Role)
	if out.MultiCurrency {
		rates, err := c.deps.FX.ListForTrip(ctx, *tc)
		if err != nil {
			return nil, err
		}
		out.ExchangeRates = exchangeRates(fxdomain.NewRateTable(rates), rateCurrencies(rates, tc.Trip.BaseCurrency), tc.Trip.BaseCurrency)
	}
	out.Participants = make([]participantOut, 0, len(parts))
	for _, part := range parts {
		out.Participants = append(out.Participants, participantOut{UserID: part.UserID.String(), Name: part.EffectiveName(), Role: string(part.Role)})
	}
	out.Categories = make([]categoryOut, 0, len(categories))
	for _, category := range categories {
		out.Categories = append(out.Categories, categoryOut{ID: category.ID.String(), Name: category.Name})
	}
	return &out, nil
}

// roster maps a trip's active participants by user id, for naming people in tool output and
// checking user_ids the AI sends back.
type roster map[uuid.UUID]string

func (c *controller) names(ctx context.Context, k *caller, tc *tripctx.TripContext) (roster, error) {
	parts, err := c.deps.Participants.List(ctx, k.who.UserID, tc.Trip.Code)
	if err != nil {
		return nil, err
	}
	result := make(roster, len(parts))
	for _, part := range parts {
		result[part.UserID] = part.EffectiveName()
	}
	return result, nil
}

func (r roster) of(id uuid.UUID) string {
	if name, ok := r[id]; ok {
		return name
	}
	return "former participant"
}

func (r roster) ids() []uuid.UUID {
	result := make([]uuid.UUID, 0, len(r))
	for id := range r {
		result = append(result, id)
	}
	return result
}

func (r roster) resolve(field string, raw []string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, 0, len(raw))
	seen := map[uuid.UUID]bool{}
	for _, value := range raw {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a user_id - use the user_id values from get_active_trip or get_trip", field, value)
		}
		if _, ok := r[id]; !ok {
			return nil, fmt.Errorf("%s: %s is not a participant of this trip", field, value)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result, nil
}

func (c *controller) payers(in []payerInput, names roster, currency string) ([]domainexpense.Payer, error) {
	if len(in) == 0 {
		return nil, errors.New("paid_by is required: ask the user who paid")
	}
	totals := map[uuid.UUID]decimal.Decimal{}
	order := make([]uuid.UUID, 0, len(in))
	for index, payer := range in {
		ids, err := names.resolve(fmt.Sprintf("paid_by[%d].user_id", index), []string{payer.UserID})
		if err != nil {
			return nil, err
		}
		amount, err := payer.Amount.DecimalIn(fmt.Sprintf("paid_by[%d].amount", index), currency)
		if err != nil {
			return nil, err
		}
		if _, ok := totals[ids[0]]; !ok {
			order = append(order, ids[0])
		}
		totals[ids[0]] = totals[ids[0]].Add(amount)
	}
	result := make([]domainexpense.Payer, 0, len(order))
	for _, id := range order {
		result = append(result, domainexpense.Payer{UserID: id, Amount: totals[id]})
	}
	return result, nil
}

// recentDuplicate finds an identical expense the same user saved moments ago, so an AI tool that
// retries a call does not record the purchase twice.
func (c *controller) recentDuplicate(ctx context.Context, k *caller, tc *tripctx.TripContext, day time.Time, description string, amount decimal.Decimal, currency string) *domainexpense.Expense {
	rows, _, _, err := c.deps.Expenses.List(ctx, *tc, expensedomain.Filter{DateFrom: &day, DateTo: &day, Page: 1, PerPage: 100})
	if err != nil {
		return nil
	}
	for index := range rows {
		row := rows[index]
		if row.CreatedByUserID == k.who.UserID && strings.EqualFold(row.Description, description) && row.Currency == currency &&
			row.Amount.Equal(amount) && time.Since(row.CreatedAt) < duplicateWindow {
			return &row
		}
	}
	return nil
}

func billShares(subtotals, splits []domainexpense.Split, names roster, scale int32) []billShareOut {
	items := map[uuid.UUID]decimal.Decimal{}
	for _, split := range subtotals {
		items[split.UserID] = split.Amount
	}
	result := make([]billShareOut, 0, len(splits))
	for _, split := range splits {
		subtotal := items[split.UserID]
		result = append(result, billShareOut{UserID: split.UserID.String(), Name: names.of(split.UserID),
			ItemsSubtotal: subtotal.StringFixedBank(scale), Extras: split.Amount.Sub(subtotal).StringFixedBank(scale),
			Total: split.Amount.StringFixedBank(scale)})
	}
	return result
}

// billNote keeps the itemised breakdown on the expense, since an expense stores only amounts.
func billNote(in billExpenseInput, names roster, scale int32) string {
	lines := make([]string, 0, len(in.Items)+4)
	for _, item := range in.Items {
		amount, _ := item.Amount.Decimal("")
		people := make([]string, 0, len(item.UserIDs))
		for _, raw := range item.UserIDs {
			if id, err := uuid.Parse(strings.TrimSpace(raw)); err == nil {
				people = append(people, names.of(id))
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", strings.TrimSpace(item.Name), amount.StringFixedBank(scale), strings.Join(people, ", ")))
	}
	for _, extra := range []struct {
		label string
		value Money
	}{{"Tax", in.Tax}, {"Service charge", in.ServiceCharge}, {"Discount", in.Discount}} {
		if amount, _ := extra.value.Decimal(""); !amount.IsZero() {
			lines = append(lines, fmt.Sprintf("%s %s", extra.label, amount.StringFixedBank(scale)))
		}
	}
	return strings.Join(lines, "\n")
}

func savedMessage(created *domainexpense.Expense) string {
	if created.Status == domainexpense.StatusPending {
		return "Saved. This trip needs the planner to approve expenses, so it counts once approved."
	}
	return "Saved."
}

func tripSummary(trip domaintrip.Trip) tripOut {
	out := tripOut{Code: trip.Code, Name: trip.Name, BaseCurrency: trip.BaseCurrency, MultiCurrency: trip.Settings.MultiCurrencyEnabled,
		StartDate: trip.StartDate.Format(dateLayout), EndDate: trip.EndDate.Format(dateLayout), Status: "open"}
	if trip.Country != nil {
		out.Country = *trip.Country
	}
	switch {
	case trip.IsArchived:
		out.Status = "archived"
	case trip.IsFinalized:
		out.Status = "finalized"
	}
	return out
}

func expenseSummary(row domainexpense.Expense, names roster) expenseOut {
	scale := money.DisplayScale(row.Currency)
	out := expenseOut{ID: row.ID.String(), Date: row.ExpenseDate.Format(dateLayout), Description: row.Description,
		Amount: row.Amount.StringFixedBank(scale), Currency: row.Currency, Status: string(row.Status), SplitType: string(row.SplitType),
		PaidBy: make([]amountByUser, 0, len(row.Payers)), Owed: make([]amountByUser, 0, len(row.Splits))}
	for _, payer := range row.Payers {
		out.PaidBy = append(out.PaidBy, amountByUser{UserID: payer.UserID.String(), Name: names.of(payer.UserID), Amount: payer.Amount.StringFixedBank(scale)})
	}
	for _, split := range row.Splits {
		out.Owed = append(out.Owed, amountByUser{UserID: split.UserID.String(), Name: names.of(split.UserID), Amount: split.Amount.StringFixedBank(scale)})
	}
	if row.Note != nil {
		out.Note = *row.Note
	}
	if row.CreatedVia != nil {
		out.CreatedVia = *row.CreatedVia
	}
	return out
}

func settlementSummary(row domainsettlement.Settlement, names roster) settlementOut {
	out := settlementOut{ID: row.ID.String(), Date: row.SettlementDate.Format(dateLayout), From: names.of(row.FromUserID), To: names.of(row.ToUserID),
		Amount: row.Amount.StringFixedBank(money.DisplayScale(row.Currency)), Currency: row.Currency, Method: string(row.Method), Status: string(row.Status)}
	if row.Note != nil {
		out.Note = *row.Note
	}
	return out
}

func parseDate(field, value string) (time.Time, error) {
	parsed, err := time.Parse(dateLayout, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be a date like 2026-10-02, got %q", field, value)
	}
	return parsed, nil
}

func dateOnly(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func optionalUUID(field, value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be an id from get_trip, got %q", field, value)
	}
	return &id, nil
}
