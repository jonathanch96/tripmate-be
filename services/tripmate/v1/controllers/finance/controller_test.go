package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
)

var (
	tripID   = uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	actorID  = uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	otherID  = uuid.MustParse("00000000-0000-0000-0000-0000000000b2")
	rowID    = uuid.MustParse("00000000-0000-0000-0000-0000000000c1")
	tripCode = "ABC123"
)

type fakeTrips struct{ trip domaintrip.Trip }

func (f fakeTrips) Create(context.Context, uuid.UUID, tripdomain.CreateInput) (*domaintrip.Trip, error) {
	return nil, nil
}
func (f fakeTrips) GetByCode(context.Context, uuid.UUID, string) (*domaintrip.Trip, error) {
	value := f.trip
	return &value, nil
}
func (f fakeTrips) ListMine(context.Context, uuid.UUID, tripdomain.ListFilter) ([]domaintrip.Trip, int64, error) {
	return nil, 0, nil
}
func (f fakeTrips) UpdateSettings(context.Context, uuid.UUID, string, tripdomain.UpdateSettingsInput) (*domaintrip.Trip, error) {
	return nil, nil
}
func (f fakeTrips) FindByCode(context.Context, string) (*domaintrip.Trip, error) {
	value := f.trip
	return &value, nil
}

type fakeParts struct{ member domainparticipant.Participant }

func (f fakeParts) Join(context.Context, uuid.UUID, string) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (f fakeParts) Add(context.Context, uuid.UUID, string, uuid.UUID) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (f fakeParts) List(context.Context, uuid.UUID, string) ([]domainparticipant.Participant, error) {
	return nil, nil
}
func (f fakeParts) Update(context.Context, uuid.UUID, string, uuid.UUID, *domainparticipant.BankInfo, *domainparticipant.Role, *string) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (f fakeParts) Remove(context.Context, uuid.UUID, string, uuid.UUID) error { return nil }
func (f fakeParts) GetMembership(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	value := f.member
	return &value, nil
}

type fakeBalances struct {
	err   error
	calls int
}

func (f *fakeBalances) Calculate(context.Context, tripctx.TripContext) (*domainbalance.Result, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &domainbalance.Result{BaseCurrency: "PHP",
		Balances: []domainbalance.ParticipantBalance{{UserID: actorID, User: domainuser.PublicUser{ID: actorID, Name: "Ana"},
			TotalPaid: decimal.RequireFromString("120.5"), TotalOwed: decimal.RequireFromString("40"), NetBalance: decimal.RequireFromString("80.5")}},
		Debts:     []domainbalance.Transfer{{FromUserID: otherID, ToUserID: actorID, Amount: decimal.RequireFromString("80.5"), Currency: "PHP"}},
		Summary:   domainbalance.Summary{TotalExpenses: decimal.RequireFromString("120.5"), UnsettledTotal: decimal.RequireFromString("80.5"), ExpenseCount: 1},
		RatesUsed: []domainbalance.AppliedRate{{From: "USD", To: "PHP", Rate: decimal.NewFromInt(50), IsFinal: true}}}, nil
}
func (f *fakeBalances) Summary(context.Context, tripctx.TripContext) (*domainbalance.Summary, error) {
	return nil, nil
}
func (f *fakeBalances) Ledger(context.Context, tripctx.TripContext, balancedomain.LedgerFilter) (*domainbalance.Ledger, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &domainbalance.Ledger{BaseCurrency: "PHP", MemberUserID: actorID, NetBalance: decimal.RequireFromString("80.5")}, nil
}
func (f *fakeBalances) OutstandingDebt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (decimal.Decimal, error) {
	return decimal.Zero, nil
}
func (f *fakeBalances) FinalSettlement(context.Context, tripctx.TripContext) (*domainbalance.FinalPlan, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	bank, number, holder := "Bank", "12345678", "Ana"
	return &domainbalance.FinalPlan{BaseCurrency: "PHP", Transfers: []domainbalance.Transfer{{FromUserID: otherID, ToUserID: actorID,
		Amount: decimal.RequireFromString("80.5"), Currency: "PHP", BankName: &bank, BankAccountNumber: &number, BankAccountHolder: &holder}}}, nil
}

type fakeSettlements struct {
	err                            error
	records, approvals, rejections int
	deletes                        int
	lastFilter                     settlementdomain.Filter
	lastInput                      settlementdomain.RecordInput
	lastReason                     string
	statusOnRecord                 domainsettlement.Status
}

func (f *fakeSettlements) row(status domainsettlement.Status) *domainsettlement.Settlement {
	number := "12345678"
	return &domainsettlement.Settlement{ID: rowID, TripID: tripID, FromUserID: otherID, ToUserID: actorID,
		Amount: decimal.RequireFromString("50"), Currency: "PHP", Method: domainsettlement.MethodBankTransfer,
		BankAccountNumber: &number, Status: status, Version: 1,
		FromUser:  &domainuser.PublicUser{ID: otherID, Name: "Ben"},
		ToUser:    &domainuser.PublicUser{ID: actorID, Name: "Ana"},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}
func (f *fakeSettlements) Record(_ context.Context, _ identity.Identity, _ tripctx.TripContext, in settlementdomain.RecordInput) (*domainsettlement.Settlement, error) {
	f.records++
	f.lastInput = in
	if f.err != nil {
		return nil, f.err
	}
	return f.row(f.statusOnRecord), nil
}
func (f *fakeSettlements) Approve(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) (*domainsettlement.Settlement, error) {
	f.approvals++
	if f.err != nil {
		return nil, f.err
	}
	return f.row(domainsettlement.StatusApproved), nil
}
func (f *fakeSettlements) Reject(_ context.Context, _ identity.Identity, _ tripctx.TripContext, _ uuid.UUID, reason string) (*domainsettlement.Settlement, error) {
	f.rejections++
	f.lastReason = reason
	if f.err != nil {
		return nil, f.err
	}
	return f.row(domainsettlement.StatusRejected), nil
}
func (f *fakeSettlements) List(_ context.Context, _ tripctx.TripContext, filter settlementdomain.Filter) ([]domainsettlement.Settlement, int64, error) {
	f.lastFilter = filter
	if f.err != nil {
		return nil, 0, f.err
	}
	return []domainsettlement.Settlement{*f.row(domainsettlement.StatusApproved)}, 42, nil
}
func (f *fakeSettlements) Delete(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) error {
	f.deletes++
	return f.err
}

type fakeFX struct {
	err        error
	sets       int
	lastRate   fxdomain.SetRateInput
	deletes    int
	lastDelete [2]string
}

func (f *fakeFX) EffectiveTable(context.Context, uuid.UUID) (*fxdomain.RateTable, error) {
	return fxdomain.NewRateTable(nil), nil
}
func (f *fakeFX) SetTripRate(_ context.Context, _ identity.Identity, _ tripctx.TripContext, in fxdomain.SetRateInput) (*domainfx.Rate, error) {
	f.sets++
	f.lastRate = in
	if f.err != nil {
		return nil, f.err
	}
	return &domainfx.Rate{ID: uuid.New(), TripID: &tripID, FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.NewFromInt(50), IsFinal: true, Source: domainfx.SourceManual}, nil
}
func (f *fakeFX) DeleteTripRate(_ context.Context, _ identity.Identity, _ tripctx.TripContext, from, to string) error {
	f.deletes++
	f.lastDelete = [2]string{from, to}
	return f.err
}
func (f *fakeFX) ListForTrip(context.Context, tripctx.TripContext) ([]domainfx.Rate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []domainfx.Rate{{ID: uuid.New(), TripID: &tripID, FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.NewFromInt(50), IsFinal: true, Source: domainfx.SourceManual}}, nil
}
func (f *fakeFX) LockAll(context.Context, uuid.UUID) error { return nil }
func (f *fakeFX) ListGlobal(context.Context) ([]domainfx.Rate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []domainfx.Rate{{ID: uuid.New(), FromCurrency: "USD", ToCurrency: "PHP", Rate: decimal.NewFromInt(50), Source: domainfx.SourceSeed}}, nil
}

type fakeFinal struct {
	finalizes, unfinalizes    int
	balances                  *fakeBalances
	finalizeErr, unfinalizErr error
}

func (f *fakeFinal) Finalize(ctx context.Context, _ identity.Identity, tc tripctx.TripContext) (*domainbalance.FinalPlan, error) {
	f.finalizes++
	if f.finalizeErr != nil {
		return nil, f.finalizeErr
	}
	return f.balances.FinalSettlement(ctx, tc)
}
func (f *fakeFinal) Unfinalize(context.Context, identity.Identity, tripctx.TripContext) error {
	f.unfinalizes++
	return f.unfinalizErr
}

type harness struct {
	engine      *gin.Engine
	balances    *fakeBalances
	settlements *fakeSettlements
	fx          *fakeFX
	final       *fakeFinal
}

func newHarness(role domainparticipant.Role) *harness {
	gin.SetMode(gin.TestMode)
	trip := domaintrip.Trip{ID: tripID, Code: tripCode, BaseCurrency: "PHP", StartDate: time.Now(), EndDate: time.Now()}
	member := domainparticipant.Participant{TripID: tripID, UserID: actorID, Role: role}
	balances := &fakeBalances{}
	h := &harness{balances: balances, settlements: &fakeSettlements{statusOnRecord: domainsettlement.StatusPending},
		fx: &fakeFX{}, final: &fakeFinal{balances: balances}}
	engine := gin.New()
	engine.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(identity.WithContext(ctx.Request.Context(), identity.Identity{UserID: actorID}))
		ctx.Next()
	})
	NewController(fakeTrips{trip: trip}, fakeParts{member: member}, balances, h.settlements, h.fx, h.final).RegisterRoutes(engine.Group("/api/v1"))
	h.engine = engine
	return h
}

func (h *harness) do(method, path string, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.engine.ServeHTTP(recorder, request)
	return recorder
}

func decode(t *testing.T, recorder *httptest.ResponseRecorder) response.Envelope {
	t.Helper()
	var envelope response.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body is not an envelope: %s", recorder.Body)
	}
	return envelope
}

// Every route in the sprint's §E-09 table, with the success code the spec names.
func TestEveryRouteIsRegisteredWithItsSuccessCode(t *testing.T) {
	for _, route := range []struct {
		name, method, path, body, code string
		status                         int
	}{
		{name: "balances", method: http.MethodGet, path: "/api/v1/trips/ABC123/balances", code: "BALANCES_FETCHED", status: http.StatusOK},
		{name: "final settlement", method: http.MethodGet, path: "/api/v1/trips/ABC123/final-settlement", code: "FINAL_SETTLEMENT_FETCHED", status: http.StatusOK},
		{name: "list settlements", method: http.MethodGet, path: "/api/v1/trips/ABC123/settlements", code: "SETTLEMENTS_FETCHED", status: http.StatusOK},
		{name: "create settlement", method: http.MethodPost, path: "/api/v1/trips/ABC123/settlements",
			body: `{"from_user_id":"00000000-0000-0000-0000-0000000000b2","to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"50","currency":"PHP","method":"cash"}`,
			code: "SETTLEMENT_CREATED", status: http.StatusCreated},
		{name: "trip rates", method: http.MethodGet, path: "/api/v1/trips/ABC123/exchange-rates", code: "RATES_FETCHED", status: http.StatusOK},
		{name: "global rates", method: http.MethodGet, path: "/api/v1/exchange-rates", code: "RATES_FETCHED", status: http.StatusOK},
		{name: "approve", method: http.MethodPost, path: "/api/v1/trips/ABC123/settlements/" + rowID.String() + "/approve", code: "SETTLEMENT_APPROVED", status: http.StatusOK},
		{name: "reject", method: http.MethodPost, path: "/api/v1/trips/ABC123/settlements/" + rowID.String() + "/reject", body: `{"reason":"duplicate"}`, code: "SETTLEMENT_REJECTED", status: http.StatusOK},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/trips/ABC123/settlements/" + rowID.String(), code: "SETTLEMENT_DELETED", status: http.StatusOK},
		{name: "set rate", method: http.MethodPut, path: "/api/v1/trips/ABC123/exchange-rates", body: `{"from":"USD","to":"PHP","rate":"50"}`, code: "RATE_SET", status: http.StatusOK},
		{name: "finalize", method: http.MethodPost, path: "/api/v1/trips/ABC123/finalize", code: "TRIP_FINALIZED", status: http.StatusOK},
		{name: "unfinalize", method: http.MethodPost, path: "/api/v1/trips/ABC123/unfinalize", code: "TRIP_UNFINALIZED", status: http.StatusOK},
	} {
		t.Run(route.name, func(t *testing.T) {
			recorder := newHarness(domainparticipant.RolePlanner).do(route.method, route.path, route.body)
			envelope := decode(t, recorder)
			if recorder.Code != route.status || envelope.Code != route.code {
				t.Fatalf("response = %d/%s, want %d/%s (body %s)", recorder.Code, envelope.Code, route.status, route.code, recorder.Body)
			}
		})
	}
}

func TestPlannerOnlyRoutesRejectMembers(t *testing.T) {
	for _, route := range []struct {
		name, method, path, body string
	}{
		{name: "approve", method: http.MethodPost, path: "/api/v1/trips/ABC123/settlements/" + rowID.String() + "/approve"},
		{name: "reject", method: http.MethodPost, path: "/api/v1/trips/ABC123/settlements/" + rowID.String() + "/reject", body: `{"reason":"duplicate"}`},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/trips/ABC123/settlements/" + rowID.String()},
		{name: "set rate", method: http.MethodPut, path: "/api/v1/trips/ABC123/exchange-rates", body: `{"from":"USD","to":"PHP","rate":"50"}`},
		{name: "finalize", method: http.MethodPost, path: "/api/v1/trips/ABC123/finalize"},
		{name: "unfinalize", method: http.MethodPost, path: "/api/v1/trips/ABC123/unfinalize"},
	} {
		t.Run(route.name, func(t *testing.T) {
			h := newHarness(domainparticipant.RoleParticipant)
			recorder := h.do(route.method, route.path, route.body)
			envelope := decode(t, recorder)
			if recorder.Code != http.StatusForbidden || envelope.Code != "PLANNER_ONLY" {
				t.Fatalf("response = %d/%s, want 403/PLANNER_ONLY", recorder.Code, envelope.Code)
			}
			calls := h.settlements.approvals + h.settlements.rejections + h.settlements.deletes + h.fx.sets + h.final.finalizes + h.final.unfinalizes
			if calls != 0 {
				t.Fatalf("the domain was reached %d times behind a planner-only guard", calls)
			}
		})
	}
}

func TestBalancesResponseIsFullyTypedAndMoneyIsAString(t *testing.T) {
	recorder := newHarness(domainparticipant.RolePlanner).do(http.MethodGet, "/api/v1/trips/ABC123/balances", "")
	var body struct {
		Data struct {
			BaseCurrency string `json:"base_currency"`
			Balances     []struct {
				UserID     string `json:"user_id"`
				NetBalance string `json:"net_balance"`
				TotalPaid  string `json:"total_paid"`
				User       struct {
					Name string `json:"name"`
				} `json:"user"`
			} `json:"balances"`
			Debts []struct {
				Amount string `json:"amount"`
			} `json:"debts"`
			Summary struct {
				TotalExpenses  string `json:"total_expenses"`
				UnsettledTotal string `json:"unsettled_total"`
				ExpenseCount   int    `json:"expense_count"`
			} `json:"summary"`
			RatesUsed []struct {
				From    string `json:"from"`
				Rate    string `json:"rate"`
				IsFinal bool   `json:"is_final"`
			} `json:"rates_used"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || body.Data.BaseCurrency != "PHP" || len(body.Data.Balances) != 1 {
		t.Fatalf("response = %d body = %s", recorder.Code, recorder.Body)
	}
	// Money crosses the wire as a scaled string, never a JSON number.
	if body.Data.Balances[0].NetBalance != "80.50" || body.Data.Balances[0].TotalPaid != "120.50" || body.Data.Summary.UnsettledTotal != "80.50" {
		t.Fatalf("money is not scaled strings: %s", recorder.Body)
	}
	if body.Data.Balances[0].User.Name != "Ana" || len(body.Data.Debts) != 1 || body.Data.Debts[0].Amount != "80.50" {
		t.Fatalf("nested rows are wrong: %s", recorder.Body)
	}
	if len(body.Data.RatesUsed) != 1 || body.Data.RatesUsed[0].From != "USD" || !body.Data.RatesUsed[0].IsFinal {
		t.Fatalf("rates_used is wrong: %s", recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), `"net_balance":80.5`) {
		t.Fatalf("money was serialized as a float: %s", recorder.Body)
	}
}

func TestSettlementResponseMasksTheStoredAccountNumber(t *testing.T) {
	recorder := newHarness(domainparticipant.RolePlanner).do(http.MethodGet, "/api/v1/trips/ABC123/settlements", "")
	var body struct {
		Data []struct {
			BankAccountNumber *string `json:"bank_account_number"`
			Amount            string  `json:"amount"`
			FromUser          struct {
				Name string `json:"name"`
			} `json:"from_user"`
		} `json:"data"`
		Meta struct {
			Pagination struct {
				Page       int   `json:"page"`
				PerPage    int   `json:"per_page"`
				TotalItems int64 `json:"total_items"`
				TotalPages int   `json:"total_pages"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].BankAccountNumber == nil {
		t.Fatalf("body = %s", recorder.Body)
	}
	if *body.Data[0].BankAccountNumber == "12345678" {
		t.Fatalf("the raw account number reached the client: %s", recorder.Body)
	}
	if body.Data[0].Amount != "50.00" || body.Data[0].FromUser.Name != "Ben" {
		t.Fatalf("row = %s", recorder.Body)
	}
	if body.Meta.Pagination.TotalItems != 42 || body.Meta.Pagination.TotalPages != 3 || body.Meta.Pagination.PerPage != 20 {
		t.Fatalf("pagination = %+v", body.Meta.Pagination)
	}
}

func TestListAppliesQueryParametersAndClampsThem(t *testing.T) {
	for _, test := range []struct {
		name, query       string
		wantPage, wantPer int
		wantStatus        string
	}{
		{name: "defaults", query: "", wantPage: 1, wantPer: 20},
		{name: "explicit page", query: "?page=3&per_page=5", wantPage: 3, wantPer: 5},
		{name: "status filter", query: "?status=pending", wantPage: 1, wantPer: 20, wantStatus: "pending"},
		{name: "per_page above the cap falls back", query: "?per_page=5000", wantPage: 1, wantPer: 20},
		{name: "negative page is clamped", query: "?page=-4", wantPage: 1, wantPer: 20},
		{name: "garbage page is clamped", query: "?page=abc", wantPage: 1, wantPer: 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(domainparticipant.RolePlanner)
			if recorder := h.do(http.MethodGet, "/api/v1/trips/ABC123/settlements"+test.query, ""); recorder.Code != http.StatusOK {
				t.Fatalf("response = %d", recorder.Code)
			}
			if h.settlements.lastFilter.Page != test.wantPage || h.settlements.lastFilter.PerPage != test.wantPer {
				t.Fatalf("filter = %+v, want page %d per_page %d", h.settlements.lastFilter, test.wantPage, test.wantPer)
			}
			if test.wantStatus == "" && h.settlements.lastFilter.Status != nil {
				t.Fatalf("status filter = %v, want none", *h.settlements.lastFilter.Status)
			}
			if test.wantStatus != "" && (h.settlements.lastFilter.Status == nil || string(*h.settlements.lastFilter.Status) != test.wantStatus) {
				t.Fatalf("status filter = %v, want %s", h.settlements.lastFilter.Status, test.wantStatus)
			}
		})
	}
}

func TestCreateRejectsMalformedBodiesBeforeReachingTheDomain(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{name: "not json", body: `{`},
		{name: "missing from_user_id", body: `{"to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"50","currency":"PHP","method":"cash"}`},
		{name: "from_user_id is not a uuid", body: `{"from_user_id":"nope","to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"50","currency":"PHP","method":"cash"}`},
		{name: "amount is not a decimal", body: `{"from_user_id":"00000000-0000-0000-0000-0000000000b2","to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"lots","currency":"PHP","method":"cash"}`},
		{name: "unknown method", body: `{"from_user_id":"00000000-0000-0000-0000-0000000000b2","to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"50","currency":"PHP","method":"crypto"}`},
		{name: "currency is not three letters", body: `{"from_user_id":"00000000-0000-0000-0000-0000000000b2","to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"50","currency":"PESO","method":"cash"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(domainparticipant.RolePlanner)
			recorder := h.do(http.MethodPost, "/api/v1/trips/ABC123/settlements", test.body)
			envelope := decode(t, recorder)
			if recorder.Code != http.StatusBadRequest || envelope.Code != "VALIDATION_FAILED" {
				t.Fatalf("response = %d/%s, want 400/VALIDATION_FAILED", recorder.Code, envelope.Code)
			}
			if h.settlements.records != 0 {
				t.Fatal("a malformed body reached the domain")
			}
		})
	}
}

func TestRejectRequiresAReasonBody(t *testing.T) {
	h := newHarness(domainparticipant.RolePlanner)
	recorder := h.do(http.MethodPost, "/api/v1/trips/ABC123/settlements/"+rowID.String()+"/reject", `{}`)
	if envelope := decode(t, recorder); recorder.Code != http.StatusBadRequest || envelope.Code != "VALIDATION_FAILED" {
		t.Fatalf("response = %d/%s, want 400/VALIDATION_FAILED", recorder.Code, envelope.Code)
	}
	if h.settlements.rejections != 0 {
		t.Fatal("a reasonless rejection reached the domain")
	}
	h = newHarness(domainparticipant.RolePlanner)
	if recorder = h.do(http.MethodPost, "/api/v1/trips/ABC123/settlements/"+rowID.String()+"/reject", `{"reason":"duplicate entry"}`); recorder.Code != http.StatusOK {
		t.Fatalf("response = %d body = %s", recorder.Code, recorder.Body)
	}
	if h.settlements.lastReason != "duplicate entry" {
		t.Fatalf("reason = %q, want it passed through verbatim", h.settlements.lastReason)
	}
}

func TestUnparseableSettlementIDIsANotFound(t *testing.T) {
	h := newHarness(domainparticipant.RolePlanner)
	recorder := h.do(http.MethodPost, "/api/v1/trips/ABC123/settlements/not-a-uuid/approve", "")
	if envelope := decode(t, recorder); recorder.Code != http.StatusNotFound || envelope.Code != "SETTLEMENT_NOT_FOUND" {
		t.Fatalf("response = %d/%s, want 404/SETTLEMENT_NOT_FOUND", recorder.Code, envelope.Code)
	}
	if h.settlements.approvals != 0 {
		t.Fatal("an unparseable id reached the domain")
	}
}

// Domain errors keep their code and HTTP status through the controller — the money guards in
// particular, since the frontend renders them as inline form errors rather than toasts.
func TestDomainErrorCodesSurviveTheController(t *testing.T) {
	for _, test := range []struct {
		code   string
		status int
	}{
		// Statuses per 02-architecture-and-standards.md §error catalog.
		{code: "SETTLEMENT_EXCEEDS_DEBT", status: http.StatusBadRequest},
		{code: "SETTLEMENT_NOT_ALLOWED_YET", status: http.StatusForbidden},
		{code: "INVALID_CURRENCY", status: http.StatusBadRequest},
		{code: "TRIP_FINALIZED", status: http.StatusForbidden},
	} {
		t.Run(test.code, func(t *testing.T) {
			h := newHarness(domainparticipant.RolePlanner)
			h.settlements.err = apperror.New(test.code)
			recorder := h.do(http.MethodPost, "/api/v1/trips/ABC123/settlements",
				`{"from_user_id":"00000000-0000-0000-0000-0000000000b2","to_user_id":"00000000-0000-0000-0000-0000000000b1","amount":"50","currency":"PHP","method":"cash"}`)
			envelope := decode(t, recorder)
			if envelope.Code != test.code {
				t.Fatalf("code = %s, want %s", envelope.Code, test.code)
			}
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d for %s", recorder.Code, test.status, test.code)
			}
		})
	}
}

func TestMissingRatesSurfaceAsExchangeRateMissingOnBalances(t *testing.T) {
	h := newHarness(domainparticipant.RolePlanner)
	h.balances.err = apperror.Newf("EXCHANGE_RATE_MISSING", "Exchange rates required: %s", "USD→PHP")
	recorder := h.do(http.MethodGet, "/api/v1/trips/ABC123/balances", "")
	envelope := decode(t, recorder)
	if envelope.Code != "EXCHANGE_RATE_MISSING" || !strings.Contains(envelope.Message, "USD→PHP") {
		t.Fatalf("envelope = %+v — the frontend needs the missing pairs to prefill its fix-it form", envelope)
	}
}

func TestFinalSettlementCarriesBankDetailsThroughUnchanged(t *testing.T) {
	recorder := newHarness(domainparticipant.RolePlanner).do(http.MethodGet, "/api/v1/trips/ABC123/final-settlement", "")
	var body struct {
		Data struct {
			BaseCurrency string `json:"base_currency"`
			Transfers    []struct {
				Amount            string  `json:"amount"`
				BankName          *string `json:"bank_name"`
				BankAccountNumber *string `json:"bank_account_number"`
			} `json:"transfers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Transfers) != 1 || body.Data.Transfers[0].BankAccountNumber == nil {
		t.Fatalf("body = %s", recorder.Body)
	}
	// The domain already decides who may see this; the controller must not re-mask it, or the
	// payer cannot read the account they are meant to pay.
	if *body.Data.Transfers[0].BankAccountNumber != "12345678" || body.Data.Transfers[0].Amount != "80.50" {
		t.Fatalf("transfer = %s", recorder.Body)
	}
}

func TestSetRatePassesTheParsedRateToTheDomain(t *testing.T) {
	h := newHarness(domainparticipant.RolePlanner)
	if recorder := h.do(http.MethodPut, "/api/v1/trips/ABC123/exchange-rates", `{"from":"usd","to":"php","rate":"56.125"}`); recorder.Code != http.StatusOK {
		t.Fatalf("response = %d body = %s", recorder.Code, recorder.Body)
	}
	if h.fx.lastRate.FromCurrency != "usd" || !h.fx.lastRate.Rate.Equal(decimal.RequireFromString("56.125")) {
		t.Fatalf("rate input = %+v", h.fx.lastRate)
	}
	h = newHarness(domainparticipant.RolePlanner)
	if recorder := h.do(http.MethodPut, "/api/v1/trips/ABC123/exchange-rates", `{"from":"USD","to":"PHP","rate":"not-a-rate"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("response = %d, want 400", recorder.Code)
	}
	if h.fx.sets != 0 {
		t.Fatal("an unparseable rate reached the domain")
	}
}

func TestDeleteRateRequiresBothCurrenciesAndReachesTheDomain(t *testing.T) {
	h := newHarness(domainparticipant.RolePlanner)
	if recorder := h.do(http.MethodDelete, "/api/v1/trips/ABC123/exchange-rates?from=USD&to=PHP", ""); recorder.Code != http.StatusOK {
		t.Fatalf("response = %d body = %s", recorder.Code, recorder.Body)
	}
	if h.fx.deletes != 1 || h.fx.lastDelete != [2]string{"USD", "PHP"} {
		t.Fatalf("delete input = %+v", h.fx.lastDelete)
	}
	h = newHarness(domainparticipant.RolePlanner)
	if recorder := h.do(http.MethodDelete, "/api/v1/trips/ABC123/exchange-rates", ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("response = %d, want 400", recorder.Code)
	}
	if h.fx.deletes != 0 {
		t.Fatal("a request missing from/to reached the domain")
	}
}

func TestDeleteRateIsPlannerOnly(t *testing.T) {
	h := newHarness(domainparticipant.RoleParticipant)
	if recorder := h.do(http.MethodDelete, "/api/v1/trips/ABC123/exchange-rates?from=USD&to=PHP", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("response = %d, want 403", recorder.Code)
	}
}

// The global rate list is deliberately not under /trips/:code, so it must not require membership.
func TestGlobalRatesRouteIsNotTripScoped(t *testing.T) {
	recorder := newHarness(domainparticipant.RoleParticipant).do(http.MethodGet, "/api/v1/exchange-rates", "")
	var body struct {
		Data []struct {
			TripID *string `json:"trip_id"`
			Rate   string  `json:"rate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || len(body.Data) != 1 || body.Data[0].TripID != nil {
		t.Fatalf("response = %d body = %s", recorder.Code, recorder.Body)
	}
}
