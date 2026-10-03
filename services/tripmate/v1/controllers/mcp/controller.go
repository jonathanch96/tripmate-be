package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	appLogger "github.com/jblabs/tripmate-be/pkg/logger"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	oauthdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/oauth"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Instructions is sent to every AI tool when it connects. It carries the workflows the tools are
// built around; the same rules are repeated in the tool descriptions for clients that only read
// those.
const Instructions = `TripMate tracks shared trip expenses: who paid, who owes whom, and settling up.

Splitting a bill or receipt (the most common request):
1. Call get_active_trip with the bill's date (or the user's local date today if the bill shows none). If it returns more than one trip, ask the user which trip - never guess. If it returns none, show the trips it lists and ask.
2. Read the bill yourself. List every item with its price as printed, numbered, and list the trip participants returned by get_active_trip.
3. Ask the user who had what. An item may be shared by several people. Every item must be assigned to at least one participant.
4. Ask who paid, and how much each payer paid if more than one.
5. Call create_bill_expense with preview=true and show the user the per-person totals it returns. After the user confirms, call it again with preview=false to save.
Pass amounts exactly as printed. Pass tax, service charge and any bill-level discount separately - TripMate spreads them across people in proportion to what each person had. Apply item-level discounts to that item's amount instead.

Currencies: record every amount in the currency the user gave or the bill shows, and pass that code as currency - never convert it to the trip's base currency yourself. TripMate converts to the base currency for balances using the trip's exchange rate. If the trip has no rate for that currency, the tool says so: ask the user how much 1 unit is worth in the base currency, save it with set_exchange_rate (trip planners only), then retry the same call. If the user is not the planner, or the trip only allows its base currency, explain that and ask how they want to proceed.

Other things you can do: list trips, show balances and who owes whom, add other expenses (add_expense), record a repayment between two people (record_settlement), set an exchange rate, create a trip, and invite someone by email (trip planners only).
Always use the user_id values TripMate returns; never invent them. Amounts are plain decimal numbers in the expense's currency, without thousands separators (45000, not 45.000 or 45,000).
When a tool returns an error, nothing was saved by that call. Always tell the user what went wrong in plain words and what they can do, following the error's own instructions, instead of stopping silently.`

func NewController(deps Dependencies) Controller {
	return &controller{deps: deps}
}

// RegisterProtocolRoutes mounts the MCP endpoint (Streamable HTTP, stateless, JSON responses) at
// /mcp behind bearer-token auth issued by the OAuth server.
func (c *controller) RegisterProtocolRoutes(engine *gin.Engine) {
	server := mcp.NewServer(&mcp.Implementation{Name: "tripmate", Title: "TripMate", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: Instructions})
	c.registerTools(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: slog.Default()})
	guard := auth.RequireBearerToken(c.verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: c.deps.OAuth.Issuer() + "/.well-known/oauth-protected-resource/mcp",
	})
	// Every token TripMate issues carries tripmate.read, so the guard requires no scope of its own.
	// The 401 challenge still has to name a scope: MCP clients ask for exactly the scope it lists,
	// and listing only tripmate.read left the consent page with no "Create and edit" choice.
	challenge := fmt.Sprintf("Bearer resource_metadata=%q, scope=%q",
		c.deps.OAuth.Issuer()+"/.well-known/oauth-protected-resource/mcp", strings.Join(domainoauth.SupportedScopes, " "))
	engine.Any("/mcp", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The guard writes through challengeWriter; the MCP handler gets the original writer so
		// streaming and flushing are untouched.
		protected := guard(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
		protected.ServeHTTP(&challengeWriter{ResponseWriter: w, challenge: challenge}, r)
	})))
}

// challengeWriter replaces the bearer guard's WWW-Authenticate header on a rejection.
type challengeWriter struct {
	http.ResponseWriter
	challenge string
}

func (w *challengeWriter) WriteHeader(status int) {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		w.Header().Set("WWW-Authenticate", w.challenge)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (c *controller) verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	info, err := c.deps.OAuth.VerifyAccess(ctx, token)
	if err != nil {
		if apperror.Is(err, "OAUTH_INVALID_TOKEN") {
			return nil, auth.ErrInvalidToken
		}
		return nil, err
	}
	return &auth.TokenInfo{Scopes: info.Scopes, Expiration: info.ExpiresAt, UserID: info.User.ID.String(),
		Extra: map[string]any{accessKey: info}}, nil
}

const accessKey = "tripmate.access"

// caller is who a tool call acts as: the user behind the token, and the AI tool they connected.
type caller struct {
	who    identity.Identity
	access *oauthdomain.AccessInfo
}

func (c *controller) caller(ctx context.Context, req *mcp.CallToolRequest) (context.Context, *caller, error) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return ctx, nil, errors.New("not authenticated")
	}
	info, ok := req.Extra.TokenInfo.Extra[accessKey].(*oauthdomain.AccessInfo)
	if !ok {
		return ctx, nil, errors.New("not authenticated")
	}
	who := identity.Identity{UserID: info.User.ID, Email: info.User.Email, Name: info.User.Name}
	ctx = identity.WithContext(ctx, who)
	ctx = appLogger.WithUser(ctx, who.UserID.String())
	return ctx, &caller{who: who, access: info}, nil
}

func (k *caller) requireWrite() error {
	if !domainoauth.HasScope(k.access.Scopes, domainoauth.ScopeWrite) {
		return errors.New("nothing was saved: this TripMate connection is view-only, so it cannot create or change anything. " +
			"Tell the user, then explain how to fix it: in their AI app's connector settings, disconnect TripMate and connect it again; " +
			"on the TripMate page that opens, keep \"Create and edit\" ticked and press Allow. Then try this again with the same details")
	}
	return nil
}

func (k *caller) createdVia() *string {
	if k.access.ClientName == "" {
		return nil
	}
	name := k.access.ClientName
	return &name
}

func (c *controller) trip(ctx context.Context, k *caller, code string) (*tripctx.TripContext, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("trip_code is required - call list_trips or get_active_trip first")
	}
	return middleware.ResolveTrip(ctx, c.deps.Trips, c.deps.Participants, code, k.who.UserID, false)
}

// nextSteps tells the AI what to do about a domain error, so it explains the problem to the user
// and carries on instead of stopping at a bare error code.
var nextSteps = map[string]string{
	"VALIDATION_FAILED":          "Check the values against the tool's input description, fix them, and try again; ask the user if a value is unclear.",
	"INVALID_CURRENCY":           "Use a 3-letter ISO 4217 code (e.g. IDR, JPY, USD). If the trip only allows its base currency, tell the user and ask them for the amount in the base currency.",
	"SPLIT_SUM_MISMATCH":         "Make the per-person amounts add up exactly to the expense amount, or ask the user how to split it.",
	"PAYER_SUM_MISMATCH":         "Make the paid_by amounts add up exactly to the total, or ask the user how much each person paid.",
	"SPLIT_PERCENT_MISMATCH":     "Make the percentages add up to 100, or ask the user how to split it.",
	"SETTLEMENT_EXCEEDS_DEBT":    "Call get_balances, tell the user how much is actually owed, and ask whether to record that amount instead.",
	"NOT_TRIP_MEMBER":            "The user is not on this trip. Call list_trips and ask which trip they meant.",
	"TRIP_NOT_FOUND":             "Call list_trips and ask the user which trip they meant.",
	"PLANNER_ONLY":               "Only the trip's planner can do this. Tell the user and suggest they ask the planner, who can also do it in the TripMate app.",
	"EDIT_OWN_ONLY":              "This trip only lets people change their own records. Tell the user.",
	"TRIP_FINALIZED":             "This trip is finalized, so it no longer accepts changes. Tell the user; the planner can reopen it in TripMate.",
	"TRIP_ARCHIVED":              "This trip is archived, so it no longer accepts changes. Tell the user; the planner can unarchive it in TripMate.",
	"SETTLEMENT_NOT_ALLOWED_YET": "This trip only allows repayments after it ends. Tell the user; the planner can change this in the trip settings.",
	"PARTICIPANT_NOT_FOUND":      "Call get_trip for the current participants and their user_ids, and ask the user who they meant.",
	"USER_NOT_FOUND":             "Call get_trip for the current participants and their user_ids, and ask the user who they meant.",
	"EXPENSE_CATEGORY_NOT_FOUND": "Call get_trip for the trip's categories, or leave category_id out.",
	"ALREADY_PARTICIPANT":        "They are already on the trip; nothing else is needed.",
	"EXCHANGE_RATE_MISSING":      "Ask the user for the exchange rate and save it with set_exchange_rate (trip planners only), then try again.",
	"CONCURRENT_MODIFICATION":    "Someone changed this at the same time. Try again once.",
	"RATE_LIMITED":               "Wait a moment, then try again.",
}

// toolError turns a domain error into text the AI can act on: what went wrong (plus any
// field-level detail, without internal causes) and what to do next.
func toolError(err error) error {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return err
	}
	if appErr.Code == "INTERNAL_ERROR" {
		return errors.New("nothing was saved: TripMate hit an unexpected error. Tell the user, then try again once; if it fails again, suggest trying later in the TripMate app")
	}
	message := appErr.Message
	for _, field := range appErr.Fields {
		if field.Message != "" {
			message += "; " + field.Field + ": " + field.Message
		}
	}
	message = fmt.Sprintf("%s (%s).", message, appErr.Code)
	if step, ok := nextSteps[appErr.Code]; ok {
		message += " " + step
	}
	return errors.New(message)
}
