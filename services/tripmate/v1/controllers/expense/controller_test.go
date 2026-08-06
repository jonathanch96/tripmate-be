package expense

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	"github.com/shopspring/decimal"
)

type controllerTrips struct{ trip domaintrip.Trip }

func (f controllerTrips) Create(context.Context, uuid.UUID, tripdomain.CreateInput) (*domaintrip.Trip, error) {
	return nil, nil
}
func (f controllerTrips) GetByCode(context.Context, uuid.UUID, string) (*domaintrip.Trip, error) {
	value := f.trip
	return &value, nil
}
func (f controllerTrips) ListMine(context.Context, uuid.UUID, tripdomain.ListFilter) ([]domaintrip.Trip, int64, error) {
	return nil, 0, nil
}
func (f controllerTrips) UpdateSettings(context.Context, uuid.UUID, string, tripdomain.UpdateSettingsInput) (*domaintrip.Trip, error) {
	return nil, nil
}
func (f controllerTrips) FindByCode(context.Context, string) (*domaintrip.Trip, error) {
	value := f.trip
	return &value, nil
}

type controllerParts struct{ member domainparticipant.Participant }

func (f controllerParts) Join(context.Context, uuid.UUID, string) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (f controllerParts) Add(context.Context, uuid.UUID, string, uuid.UUID) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (f controllerParts) List(context.Context, uuid.UUID, string) ([]domainparticipant.Participant, error) {
	return nil, nil
}
func (f controllerParts) Update(context.Context, uuid.UUID, string, uuid.UUID, *domainparticipant.BankInfo, *domainparticipant.Role) (*domainparticipant.Participant, error) {
	return nil, nil
}
func (f controllerParts) Remove(context.Context, uuid.UUID, string, uuid.UUID) error { return nil }
func (f controllerParts) GetMembership(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	value := f.member
	return &value, nil
}

type controllerExpenses struct {
	approvals, rejections int
	expense               domainexpense.Expense
	totals                expensedomain.Totals
}

func (f *controllerExpenses) Create(context.Context, identity.Identity, tripctx.TripContext, expensedomain.CreateInput) (*domainexpense.Expense, error) {
	return nil, nil
}
func (f *controllerExpenses) Update(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, expensedomain.UpdateInput) (*domainexpense.Expense, error) {
	return nil, nil
}
func (f *controllerExpenses) Delete(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) error {
	return nil
}
func (f *controllerExpenses) Approve(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID) (*domainexpense.Expense, error) {
	f.approvals++
	value := f.expense
	value.Status = domainexpense.StatusApproved
	return &value, nil
}
func (f *controllerExpenses) Reject(context.Context, identity.Identity, tripctx.TripContext, uuid.UUID, string) (*domainexpense.Expense, error) {
	f.rejections++
	value := f.expense
	value.Status = domainexpense.StatusRejected
	return &value, nil
}
func (f *controllerExpenses) List(context.Context, tripctx.TripContext, expensedomain.Filter) ([]domainexpense.Expense, int64, expensedomain.Totals, error) {
	return []domainexpense.Expense{f.expense}, 1, f.totals, nil
}

func TestListReturnsPaginationAndMoneyStringTotals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	actorID, tripID := uuid.New(), uuid.New()
	trip := domaintrip.Trip{ID: tripID, Code: "ABC123", BaseCurrency: "PHP"}
	member := domainparticipant.Participant{TripID: tripID, UserID: actorID, Role: domainparticipant.RoleParticipant}
	expenses := &controllerExpenses{expense: domainexpense.Expense{ID: uuid.New(), TripID: tripID, ExpenseDate: time.Now(), Description: "Dinner", Amount: decimal.RequireFromString("22500"), Currency: "PHP", Status: domainexpense.StatusApproved, CreatedByUserID: actorID}, totals: expensedomain.Totals{ByCurrency: map[string]decimal.Decimal{"PHP": decimal.RequireFromString("22500")}, CountByStatus: map[domainexpense.Status]int64{domainexpense.StatusApproved: 1}}}
	engine := gin.New()
	engine.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(identity.WithContext(ctx.Request.Context(), identity.Identity{UserID: actorID}))
		ctx.Next()
	})
	NewController(controllerTrips{trip: trip}, controllerParts{member: member}, expenses).RegisterRoutes(engine.Group("/api/v1"))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/trips/ABC123/expenses", nil))
	var body struct {
		Meta struct {
			Totals struct {
				ByCurrency    map[string]string `json:"by_currency"`
				CountByStatus map[string]int64  `json:"count_by_status"`
			} `json:"totals"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || body.Meta.Totals.ByCurrency["PHP"] != "22500.00" || body.Meta.Totals.CountByStatus["approved"] != 1 {
		t.Fatalf("response=%d body=%s", recorder.Code, recorder.Body)
	}
}
func (f *controllerExpenses) Get(context.Context, tripctx.TripContext, uuid.UUID) (*domainexpense.Expense, error) {
	value := f.expense
	return &value, nil
}

func approvalRequest(t *testing.T, role domainparticipant.Role, suffix string, body []byte) (*httptest.ResponseRecorder, *controllerExpenses) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	actorID, tripID, expenseID := uuid.New(), uuid.New(), uuid.New()
	trip := domaintrip.Trip{ID: tripID, Code: "ABC123", BaseCurrency: "PHP", StartDate: time.Now(), EndDate: time.Now()}
	member := domainparticipant.Participant{TripID: tripID, UserID: actorID, Role: role}
	expenses := &controllerExpenses{expense: domainexpense.Expense{ID: expenseID, TripID: tripID, ExpenseDate: time.Now(), Amount: decimal.NewFromInt(10), Currency: "PHP", Status: domainexpense.StatusPending, CreatedByUserID: actorID}}
	engine := gin.New()
	engine.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(identity.WithContext(ctx.Request.Context(), identity.Identity{UserID: actorID}))
		ctx.Next()
	})
	group := engine.Group("/api/v1")
	NewController(controllerTrips{trip: trip}, controllerParts{member: member}, expenses).RegisterRoutes(group)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/ABC123/expenses/"+expenseID.String()+suffix, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder, expenses
}

func TestApproveAndRejectRoutesRequirePlanner(t *testing.T) {
	for _, route := range []struct {
		suffix string
		body   []byte
	}{{"/approve", nil}, {"/reject", []byte(`{"reason":"duplicate"}`)}} {
		t.Run(route.suffix+" participant", func(t *testing.T) {
			recorder, expenses := approvalRequest(t, domainparticipant.RoleParticipant, route.suffix, route.body)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusForbidden || envelope.Code != "PLANNER_ONLY" || expenses.approvals+expenses.rejections != 0 {
				t.Fatalf("response=%d/%s calls=%d", recorder.Code, envelope.Code, expenses.approvals+expenses.rejections)
			}
		})
		t.Run(route.suffix+" planner", func(t *testing.T) {
			recorder, expenses := approvalRequest(t, domainparticipant.RolePlanner, route.suffix, route.body)
			if recorder.Code != http.StatusOK || expenses.approvals+expenses.rejections != 1 {
				t.Fatalf("response=%d body=%s calls=%d", recorder.Code, recorder.Body, expenses.approvals+expenses.rejections)
			}
		})
	}
}
