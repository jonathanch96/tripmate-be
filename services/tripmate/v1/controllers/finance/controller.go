package finance

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	finaldomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/finalization"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainbalance "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/balance"
	domainfx "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/fx"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
	"github.com/shopspring/decimal"
	"strconv"
)

type Controller interface{ RegisterRoutes(*gin.RouterGroup) }
type controller struct {
	trips       tripdomain.Service
	parts       participantdomain.Service
	balances    balancedomain.Service
	settlements settlementdomain.Service
	fx          fxdomain.Service
	final       finaldomain.Service
}

func NewController(t tripdomain.Service, p participantdomain.Service, b balancedomain.Service, s settlementdomain.Service, f fxdomain.Service, z finaldomain.Service) Controller {
	return &controller{t, p, b, s, f, z}
}
func (c *controller) RegisterRoutes(g *gin.RouterGroup) {
	member := g.Group("/trips/:code", middleware.RequireTripMember(c.trips, c.parts))
	member.GET("/balances", c.balancesGet)
	member.GET("/final-settlement", c.finalGet)
	member.GET("/settlements", c.settlementsList)
	member.POST("/settlements", c.settlementCreate)
	member.GET("/exchange-rates", c.ratesTrip)
	planner := g.Group("/trips/:code", middleware.RequirePlanner(c.trips, c.parts))
	planner.POST("/settlements/:id/approve", c.approve)
	planner.POST("/settlements/:id/reject", c.reject)
	planner.DELETE("/settlements/:id", c.delete)
	planner.PUT("/exchange-rates", c.rateSet)
	planner.POST("/finalize", c.finalize)
	planner.POST("/unfinalize", c.unfinalize)
	g.GET("/exchange-rates", c.ratesGlobal)
}
func actor(x *gin.Context) identity.Identity { return identity.MustFromContext(x.Request.Context()) }
func tc(x *gin.Context) tripctx.TripContext  { return tripctx.MustFromContext(x.Request.Context()) }

// balancesGet godoc
// @Summary Calculate trip balances
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/balances [get]
func (c *controller) balancesGet(x *gin.Context) {
	r, e := c.balances.Calculate(x, tc(x))
	if e != nil {
		response.Error(x, e)
		return
	}
	response.OK(x, "BALANCES_FETCHED", balanceJSON(r))
}

// finalGet godoc
// @Summary Get optimized final settlement plan
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/final-settlement [get]
func (c *controller) finalGet(x *gin.Context) {
	r, e := c.balances.FinalSettlement(x, tc(x))
	if e != nil {
		response.Error(x, e)
		return
	}
	response.OK(x, "FINAL_SETTLEMENT_FETCHED", gin.H{"base_currency": r.BaseCurrency, "transfers": transfersJSON(r.Transfers)})
}

// settlementsList godoc
// @Summary List trip settlements
// @Tags settlements
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/settlements [get]
func (c *controller) settlementsList(x *gin.Context) {
	f := settlementdomain.Filter{Page: 1, PerPage: 20}
	f.Page, _ = strconv.Atoi(x.DefaultQuery("page", "1"))
	f.PerPage, _ = strconv.Atoi(x.DefaultQuery("per_page", "20"))
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 100 {
		f.PerPage = 20
	}
	if raw := x.Query("status"); raw != "" {
		v := domainsettlement.Status(raw)
		f.Status = &v
	}
	rows, total, e := c.settlements.List(x, tc(x), f)
	if e != nil {
		response.Error(x, e)
		return
	}
	out := make([]gin.H, len(rows))
	for i := range rows {
		out[i] = settlementJSON(rows[i])
	}
	response.List(x, "SETTLEMENTS_FETCHED", out, response.Pagination{Page: f.Page, PerPage: f.PerPage, TotalItems: total, TotalPages: (int(total) + f.PerPage - 1) / f.PerPage})
}

type settlementRequest struct {
	FromUserID string  `json:"from_user_id"`
	ToUserID   string  `json:"to_user_id"`
	Amount     string  `json:"amount"`
	Currency   string  `json:"currency"`
	Method     string  `json:"method"`
	Note       *string `json:"note"`
	ProofURL   *string `json:"proof_url"`
}

// settlementCreate godoc
// @Summary Record a settlement
// @Tags settlements
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param body body settlementRequest true "Settlement"
// @Success 201 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/settlements [post]
func (c *controller) settlementCreate(x *gin.Context) {
	var q settlementRequest
	if e := x.ShouldBindJSON(&q); e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	from, e := uuid.Parse(q.FromUserID)
	if e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	to, e := uuid.Parse(q.ToUserID)
	if e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	amount, e := decimal.NewFromString(q.Amount)
	if e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	row, e := c.settlements.Record(x, actor(x), tc(x), settlementdomain.RecordInput{FromUserID: from, ToUserID: to, Amount: amount, Currency: q.Currency, Method: domainsettlement.Method(q.Method), Note: q.Note, ProofURL: q.ProofURL})
	if e != nil {
		response.Error(x, e)
		return
	}
	response.Created(x, "SETTLEMENT_CREATED", settlementJSON(*row))
}
func id(x *gin.Context) (uuid.UUID, bool) {
	v, e := uuid.Parse(x.Param("id"))
	if e != nil {
		response.Error(x, apperror.New("SETTLEMENT_NOT_FOUND"))
		return uuid.Nil, false
	}
	return v, true
}

// approve godoc
// @Summary Approve a pending settlement
// @Tags settlements
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/settlements/{id}/approve [post]
func (c *controller) approve(x *gin.Context) {
	v, ok := id(x)
	if !ok {
		return
	}
	r, e := c.settlements.Approve(x, actor(x), tc(x), v)
	if e != nil {
		response.Error(x, e)
		return
	}
	response.OK(x, "SETTLEMENT_APPROVED", settlementJSON(*r))
}

// reject godoc
// @Summary Reject a pending settlement
// @Tags settlements
// @Accept json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/settlements/{id}/reject [post]
func (c *controller) reject(x *gin.Context) {
	v, ok := id(x)
	if !ok {
		return
	}
	var q struct {
		Reason string `json:"reason"`
	}
	if e := x.ShouldBindJSON(&q); e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	r, e := c.settlements.Reject(x, actor(x), tc(x), v, q.Reason)
	if e != nil {
		response.Error(x, e)
		return
	}
	response.OK(x, "SETTLEMENT_REJECTED", settlementJSON(*r))
}

// delete godoc
// @Summary Delete a non-approved settlement
// @Tags settlements
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/settlements/{id} [delete]
func (c *controller) delete(x *gin.Context) {
	v, ok := id(x)
	if !ok {
		return
	}
	if e := c.settlements.Delete(x, actor(x), tc(x), v); e != nil {
		response.Error(x, e)
		return
	}
	response.NoData(x, "SETTLEMENT_DELETED")
}

type rateRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rate string `json:"rate"`
}

// rateSet godoc
// @Summary Set a locked trip exchange rate
// @Tags finance
// @Accept json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param body body rateRequest true "Rate"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/exchange-rates [put]
func (c *controller) rateSet(x *gin.Context) {
	var q rateRequest
	if e := x.ShouldBindJSON(&q); e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	rate, e := decimal.NewFromString(q.Rate)
	if e != nil {
		response.Error(x, apperror.New("VALIDATION_FAILED"))
		return
	}
	r, e := c.fx.SetTripRate(x, actor(x), tc(x), fxdomain.SetRateInput{FromCurrency: q.From, ToCurrency: q.To, Rate: rate})
	if e != nil {
		response.Error(x, e)
		return
	}
	response.OK(x, "RATE_SET", rateJSON(*r))
}

// ratesTrip godoc
// @Summary List effective trip exchange rates
// @Tags finance
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/exchange-rates [get]
func (c *controller) ratesTrip(x *gin.Context) {
	rows, e := c.fx.ListForTrip(x, tc(x))
	if e != nil {
		response.Error(x, e)
		return
	}
	out := make([]gin.H, len(rows))
	for i := range rows {
		out[i] = rateJSON(rows[i])
	}
	response.OK(x, "RATES_FETCHED", out)
}

// ratesGlobal godoc
// @Summary List global exchange rates
// @Tags finance
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Router /exchange-rates [get]
func (c *controller) ratesGlobal(x *gin.Context) {
	rows, e := c.fx.ListGlobal(x)
	if e != nil {
		response.Error(x, e)
		return
	}
	out := make([]gin.H, len(rows))
	for i := range rows {
		out[i] = rateJSON(rows[i])
	}
	response.OK(x, "RATES_FETCHED", out)
}

// finalize godoc
// @Summary Finalize a trip and lock rates
// @Tags finance
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/finalize [post]
func (c *controller) finalize(x *gin.Context) {
	r, e := c.final.Finalize(x, actor(x), tc(x))
	if e != nil {
		response.Error(x, e)
		return
	}
	response.OK(x, "TRIP_FINALIZED", gin.H{"base_currency": r.BaseCurrency, "transfers": transfersJSON(r.Transfers)})
}

// unfinalize godoc
// @Summary Reopen a finalized trip while retaining locked rates
// @Tags finance
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Router /trips/{code}/unfinalize [post]
func (c *controller) unfinalize(x *gin.Context) {
	if e := c.final.Unfinalize(x, actor(x), tc(x)); e != nil {
		response.Error(x, e)
		return
	}
	response.NoData(x, "TRIP_UNFINALIZED")
}
func balanceJSON(r *domainbalance.Result) gin.H {
	balances := make([]gin.H, len(r.Balances))
	for i, row := range r.Balances {
		balances[i] = gin.H{
			"user_id": row.UserID, "user": publicUserJSON(row.User),
			"total_paid":  amountJSON(row.TotalPaid, r.BaseCurrency),
			"total_owed":  amountJSON(row.TotalOwed, r.BaseCurrency),
			"net_balance": amountJSON(row.NetBalance, r.BaseCurrency),
		}
	}
	rates := make([]gin.H, len(r.RatesUsed))
	for i, rate := range r.RatesUsed {
		rates[i] = gin.H{"from": rate.From, "to": rate.To, "rate": rate.Rate.StringFixed(12), "is_final": rate.IsFinal}
	}
	return gin.H{
		"base_currency": r.BaseCurrency, "balances": balances, "debts": transfersJSON(r.Debts),
		"summary": gin.H{
			"total_expenses": amountJSON(r.Summary.TotalExpenses, r.BaseCurrency),
			"total_settled":  amountJSON(r.Summary.TotalSettled, r.BaseCurrency),
			"expense_count":  r.Summary.ExpenseCount, "settlement_count": r.Summary.SettlementCount,
			"pending_expense_count": r.Summary.PendingExpenseCount, "pending_settlement_count": r.Summary.PendingSettlementCount,
			"unsettled_total": amountJSON(r.Summary.UnsettledTotal, r.BaseCurrency),
		},
		"rates_used": rates,
	}
}
func transfersJSON(rows []domainbalance.Transfer) []gin.H {
	out := make([]gin.H, len(rows))
	for i, v := range rows {
		out[i] = gin.H{"from_user_id": v.FromUserID, "to_user_id": v.ToUserID, "amount": amountJSON(v.Amount, v.Currency), "currency": v.Currency,
			"bank_name": v.BankName, "bank_account_number": v.BankAccountNumber, "bank_account_holder": v.BankAccountHolder}
	}
	return out
}
func settlementJSON(v domainsettlement.Settlement) gin.H {
	return gin.H{"id": v.ID, "trip_id": v.TripID, "from_user_id": v.FromUserID, "to_user_id": v.ToUserID, "amount": amountJSON(v.Amount, v.Currency), "currency": v.Currency, "method": v.Method, "status": v.Status, "bank_name": v.BankName, "bank_account_number": maskedAccount(v.BankAccountNumber), "bank_account_holder": v.BankAccountHolder, "note": v.Note, "proof_url": v.ProofURL, "version": v.Version, "from_user": publicUserPtrJSON(v.FromUser), "to_user": publicUserPtrJSON(v.ToUser)}
}
func rateJSON(v domainfx.Rate) gin.H {
	return gin.H{"id": v.ID, "trip_id": v.TripID, "from": v.FromCurrency, "to": v.ToCurrency, "rate": v.Rate.String(), "is_final": v.IsFinal, "source": v.Source}
}

func amountJSON(value decimal.Decimal, currency string) string {
	return value.StringFixedBank(money.DisplayScale(currency))
}

func publicUserJSON(v domainuser.PublicUser) gin.H {
	return gin.H{"id": v.ID, "email": v.Email, "name": v.Name, "avatar_url": v.AvatarURL}
}

func publicUserPtrJSON(v *domainuser.PublicUser) any {
	if v == nil {
		return nil
	}
	return publicUserJSON(*v)
}

func maskedAccount(v *string) *string {
	if v == nil {
		return nil
	}
	masked := domainparticipant.MaskAccount(*v)
	return &masked
}
