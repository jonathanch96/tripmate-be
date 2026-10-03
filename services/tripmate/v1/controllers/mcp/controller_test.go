package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	oauthdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/oauth"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainoauth "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/oauth"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/shopspring/decimal"
)

type fakeOAuth struct{ oauthdomain.Service }

func (fakeOAuth) Issuer() string { return "https://tripmate.test" }
func (fakeOAuth) VerifyAccess(_ context.Context, token string) (*oauthdomain.AccessInfo, error) {
	if token != "good" {
		return nil, apperror.New("OAUTH_INVALID_TOKEN")
	}
	return &oauthdomain.AccessInfo{Scopes: []string{domainoauth.ScopeRead}, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

// MCP clients request exactly the scope the 401 challenge names, so it must offer write too or
// the consent page never shows "Create and edit".
func TestUnauthenticatedChallengeOffersEveryScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	(&controller{deps: Dependencies{OAuth: fakeOAuth{}}}).RegisterProtocolRoutes(engine)
	for _, auth := range []string{"", "Bearer expired"} {
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("auth %q: status = %d", auth, recorder.Code)
		}
		want := `Bearer resource_metadata="https://tripmate.test/.well-known/oauth-protected-resource/mcp", scope="tripmate.read tripmate.write"`
		if got := recorder.Header().Values("WWW-Authenticate"); len(got) != 1 || got[0] != want {
			t.Fatalf("auth %q: WWW-Authenticate = %q", auth, got)
		}
	}
	// A valid token, even a read-only one, still reaches the MCP handler.
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
	request.Header.Set("Authorization", "Bearer good")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "TripMate tracks shared trip expenses") {
		t.Fatalf("authenticated initialize: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestReadOnlyConnectionExplainsHowToReconnect(t *testing.T) {
	err := (&caller{access: &oauthdomain.AccessInfo{Scopes: []string{domainoauth.ScopeRead}}}).requireWrite()
	if err == nil || !strings.Contains(err.Error(), "nothing was saved") || !strings.Contains(err.Error(), "Create and edit") {
		t.Fatalf("err = %v", err)
	}
	if err := (&caller{access: &oauthdomain.AccessInfo{Scopes: domainoauth.SupportedScopes}}).requireWrite(); err != nil {
		t.Fatalf("write scope refused: %v", err)
	}
}

func TestToolErrorSaysWhatToDoNext(t *testing.T) {
	err := toolError(apperror.New("PLANNER_ONLY"))
	if !strings.Contains(err.Error(), "(PLANNER_ONLY).") || !strings.Contains(err.Error(), "ask the planner") {
		t.Fatalf("err = %v", err)
	}
	if err := toolError(apperror.New("INTERNAL_ERROR")); !strings.Contains(err.Error(), "Tell the user") {
		t.Fatalf("internal err = %v", err)
	}
}

type fakeFX struct {
	fxdomain.Service
	rates []domainfx.Rate
}

func (f fakeFX) EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error) {
	return fxdomain.NewRateTable(f.rates), nil
}

type fakeParticipants struct {
	participantdomain.Service
	rows []domainparticipant.Participant
}

func (f fakeParticipants) List(context.Context, uuid.UUID, string) ([]domainparticipant.Participant, error) {
	return f.rows, nil
}

func TestCurrencyForKeepsTheGivenCurrencyAndNeedsARate(t *testing.T) {
	ctx := context.Background()
	planner, member := "Ana", "Budi"
	trip := domaintrip.Trip{ID: uuid.New(), Code: "p0bCFl", Name: "Bali", BaseCurrency: "IDR",
		Settings: domaintrip.Settings{MultiCurrencyEnabled: true}}
	people := fakeParticipants{rows: []domainparticipant.Participant{
		{UserID: uuid.New(), Role: domainparticipant.RolePlanner, DisplayName: &planner},
		{UserID: uuid.New(), Role: domainparticipant.RoleParticipant, DisplayName: &member},
	}}
	jpy := domainfx.Rate{TripID: &trip.ID, FromCurrency: "JPY", ToCurrency: "IDR", Rate: decimal.NewFromInt(105)}
	usd := domainfx.Rate{FromCurrency: "IDR", ToCurrency: "USD", Rate: decimal.RequireFromString("0.00005")}
	c := &controller{deps: Dependencies{FX: fakeFX{rates: []domainfx.Rate{jpy, usd}}, Participants: people}}
	k := &caller{who: identity.Identity{UserID: people.rows[1].UserID}}
	asPlanner := &tripctx.TripContext{Trip: trip, Participant: people.rows[0]}
	asMember := &tripctx.TripContext{Trip: trip, Participant: people.rows[1]}

	if conv, err := c.currencyFor(ctx, k, asMember, ""); err != nil || conv.Currency != "IDR" || conv.out(decimal.NewFromInt(5)) != nil {
		t.Fatalf("empty currency: %+v, %v", conv, err)
	}
	conv, err := c.currencyFor(ctx, k, asMember, " jpy ")
	if err != nil || conv.Currency != "JPY" {
		t.Fatalf("jpy: %+v, %v", conv, err)
	}
	if got := conv.out(decimal.NewFromInt(3)); got == nil || got.Amount != "315" || got.Rate != "1 JPY = 105 IDR" {
		t.Fatalf("jpy conversion = %+v", got)
	}
	// A rate stored the other way round still converts.
	if conv, err := c.currencyFor(ctx, k, asMember, "USD"); err != nil || conv.out(decimal.NewFromInt(1)).Amount != "20000" {
		t.Fatalf("usd: %+v, %v", conv, err)
	}

	_, err = c.currencyFor(ctx, k, asPlanner, "SGD")
	if err == nil || !strings.Contains(err.Error(), "set_exchange_rate") || !strings.Contains(err.Error(), "1 SGD") {
		t.Fatalf("planner missing rate: %v", err)
	}
	_, err = c.currencyFor(ctx, k, asMember, "SGD")
	if err == nil || strings.Contains(err.Error(), "save it with set_exchange_rate") || !strings.Contains(err.Error(), "the trip planner (Ana)") {
		t.Fatalf("member missing rate: %v", err)
	}

	single := *asPlanner
	single.Trip.Settings.MultiCurrencyEnabled = false
	if _, err := c.currencyFor(ctx, k, &single, "JPY"); err == nil || !strings.Contains(err.Error(), "only records amounts in its base currency IDR") {
		t.Fatalf("single-currency trip: %v", err)
	}
	if _, err := c.currencyFor(ctx, k, asPlanner, "XYZ"); err == nil || !strings.Contains(err.Error(), "ISO 4217") {
		t.Fatalf("unknown currency: %v", err)
	}
}

func TestRateCurrenciesListsPairsWithTheBase(t *testing.T) {
	got := rateCurrencies([]domainfx.Rate{
		{FromCurrency: "JPY", ToCurrency: "IDR"}, {FromCurrency: "IDR", ToCurrency: "usd"},
		{FromCurrency: "JPY", ToCurrency: "IDR"}, {FromCurrency: "EUR", ToCurrency: "USD"},
	}, "idr")
	if strings.Join(got, ",") != "JPY,USD" {
		t.Fatalf("got %v", got)
	}
}
