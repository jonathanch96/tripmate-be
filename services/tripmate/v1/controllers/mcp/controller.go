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

Other things you can do: list trips, show balances and who owes whom, add other expenses (add_expense), record a repayment between two people (record_settlement), create a trip, and invite someone by email (trip planners only).
Always use the user_id values TripMate returns; never invent them. Amounts are plain decimal numbers in the expense's currency, without thousands separators (45000, not 45.000 or 45,000).`

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
	protected := auth.RequireBearerToken(c.verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: c.deps.OAuth.Issuer() + "/.well-known/oauth-protected-resource/mcp",
		Scopes:              []string{domainoauth.ScopeRead},
	})(handler)
	engine.Any("/mcp", gin.WrapH(protected))
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
		return errors.New("this connection is read-only. The user can reconnect TripMate and allow \"create and edit\" to make changes")
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

// toolError turns a domain error into text the AI can act on: the message plus any field-level
// detail, without internal causes.
func toolError(err error) error {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return err
	}
	if appErr.Code == "INTERNAL_ERROR" {
		return errors.New("TripMate hit an unexpected error; try again shortly")
	}
	message := appErr.Message
	for _, field := range appErr.Fields {
		if field.Message != "" {
			message += "; " + field.Field + ": " + field.Message
		}
	}
	return fmt.Errorf("%s (%s)", message, appErr.Code)
}
