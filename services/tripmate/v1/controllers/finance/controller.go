package finance

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	balancedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/balance"
	finaldomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/finalization"
	fxdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/fx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	settlementdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/settlement"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainsettlement "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/settlement"
	fxrequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/fx"
	settlementrequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/settlement"
	balanceresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/balance"
	fxresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/fx"
	settlementresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/settlement"
	"github.com/shopspring/decimal"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service, balances balancedomain.Service, settlements settlementdomain.Service, fx fxdomain.Service, final finaldomain.Service) Controller {
	return &controller{trips: trips, parts: parts, balances: balances, settlements: settlements, fx: fx, final: final}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	member := group.Group("/trips/:code", middleware.RequireTripMember(c.trips, c.parts))
	member.GET("/balances", c.balancesGet)
	member.GET("/ledger", c.ledgerGet)
	member.GET("/final-settlement", c.finalGet)
	member.GET("/settlements", c.settlementsList)
	member.POST("/settlements", c.settlementCreate)
	member.GET("/exchange-rates", c.ratesTrip)
	planner := group.Group("/trips/:code", middleware.RequirePlanner(c.trips, c.parts))
	planner.PATCH("/settlements/:id", c.settlementUpdate)
	planner.POST("/settlements/:id/approve", c.approve)
	planner.POST("/settlements/:id/reject", c.reject)
	planner.DELETE("/settlements/:id", c.delete)
	planner.PUT("/exchange-rates", c.rateSet)
	planner.DELETE("/exchange-rates", c.rateDelete)
	planner.POST("/finalize", c.finalize)
	planner.POST("/unfinalize", c.unfinalize)
	// Global rates are not scoped to a trip, so they are registered outside the trip group.
	group.GET("/exchange-rates", c.ratesGlobal)
}

func bind(ctx *gin.Context, value any) bool {
	if err := ctx.ShouldBindJSON(value); err != nil {
		response.Error(ctx, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "request", Rule: "invalid", Message: err.Error()}}))
		return false
	}
	return true
}

func actor(ctx *gin.Context) identity.Identity {
	return identity.MustFromContext(ctx.Request.Context())
}
func tripContext(ctx *gin.Context) tripctx.TripContext {
	return tripctx.MustFromContext(ctx.Request.Context())
}

func settlementID(ctx *gin.Context) (uuid.UUID, bool) {
	value, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("SETTLEMENT_NOT_FOUND"))
		return uuid.Nil, false
	}
	return value, true
}

// balancesGet godoc
// @Summary Calculate trip balances
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope{data=balanceresponse.Result}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/balances [get]
func (c *controller) balancesGet(ctx *gin.Context) {
	result, err := c.balances.Calculate(ctx, tripContext(ctx))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "BALANCES_FETCHED", balanceresponse.FromDomain(*result))
}

// ledgerGet godoc
// @Summary Get one member's running statement
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param member_user_id query string true "Member to build the statement for"
// @Param against_user_id query string false "Collapse every entry to the net effect against just this member"
// @Success 200 {object} response.Envelope{data=balanceresponse.Ledger}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/ledger [get]
func (c *controller) ledgerGet(ctx *gin.Context) {
	memberID, err := uuid.Parse(ctx.Query("member_user_id"))
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	filter := balancedomain.LedgerFilter{MemberUserID: memberID}
	if raw := ctx.Query("against_user_id"); raw != "" {
		against, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		filter.AgainstUserID = &against
	}
	result, err := c.balances.Ledger(ctx, tripContext(ctx), filter)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "LEDGER_FETCHED", balanceresponse.LedgerFromDomain(*result))
}

// finalGet godoc
// @Summary Get optimized final settlement plan
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope{data=balanceresponse.FinalPlan}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/final-settlement [get]
func (c *controller) finalGet(ctx *gin.Context) {
	plan, err := c.balances.FinalSettlement(ctx, tripContext(ctx))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "FINAL_SETTLEMENT_FETCHED", balanceresponse.FinalPlanFromDomain(*plan))
}

// settlementsList godoc
// @Summary List trip settlements
// @Tags settlements
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param page query int false "Page number"
// @Param per_page query int false "Rows per page"
// @Param status query string false "Filter by status" Enums(pending, approved, rejected)
// @Success 200 {object} response.Envelope{data=[]settlementresponse.Settlement}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /trips/{code}/settlements [get]
func (c *controller) settlementsList(ctx *gin.Context) {
	filter := settlementdomain.Filter{Page: 1, PerPage: 20}
	filter.Page, _ = strconv.Atoi(ctx.DefaultQuery("page", "1"))
	filter.PerPage, _ = strconv.Atoi(ctx.DefaultQuery("per_page", "20"))
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 || filter.PerPage > 100 {
		filter.PerPage = 20
	}
	if raw := ctx.Query("status"); raw != "" {
		value := domainsettlement.Status(raw)
		filter.Status = &value
	}
	rows, total, err := c.settlements.List(ctx, tripContext(ctx), filter)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.List(ctx, "SETTLEMENTS_FETCHED", settlementresponse.ListFromDomain(rows, tripContext(ctx)),
		response.Pagination{Page: filter.Page, PerPage: filter.PerPage, TotalItems: total, TotalPages: (int(total) + filter.PerPage - 1) / filter.PerPage})
}

// settlementCreate godoc
// @Summary Record a settlement
// @Tags settlements
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param body body settlementrequest.Create true "Settlement"
// @Success 201 {object} response.Envelope{data=settlementresponse.Settlement}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/settlements [post]
func (c *controller) settlementCreate(ctx *gin.Context) {
	var body settlementrequest.Create
	if !bind(ctx, &body) {
		return
	}
	from, err := uuid.Parse(body.FromUserID)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	to, err := uuid.Parse(body.ToUserID)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	amount, err := decimal.NewFromString(body.Amount)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	settledDate, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	row, err := c.settlements.Record(ctx, actor(ctx), tripContext(ctx), settlementdomain.RecordInput{FromUserID: from, ToUserID: to,
		Amount: amount, Currency: body.Currency, Method: domainsettlement.Method(body.Method), Note: body.Note, ProofURL: body.ProofURL, Date: settledDate})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "SETTLEMENT_CREATED", settlementresponse.FromDomain(*row, tripContext(ctx)))
}

// settlementUpdate godoc
// @Summary Edit a settlement
// @Tags settlements
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Param body body settlementrequest.Update true "Fields to change"
// @Success 200 {object} response.Envelope{data=settlementresponse.Settlement}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/settlements/{id} [patch]
func (c *controller) settlementUpdate(ctx *gin.Context) {
	id, ok := settlementID(ctx)
	if !ok {
		return
	}
	var body settlementrequest.Update
	if !bind(ctx, &body) {
		return
	}
	input := settlementdomain.UpdateInput{Note: body.Note, ProofURL: body.ProofURL, Version: body.Version}
	if body.Amount != nil {
		amount, amountErr := decimal.NewFromString(*body.Amount)
		if amountErr != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input.Amount = &amount
	}
	if body.Currency != nil {
		input.Currency = body.Currency
	}
	if body.Method != nil {
		method := domainsettlement.Method(*body.Method)
		input.Method = &method
	}
	if body.Date != nil {
		settledDate, dateErr := time.Parse("2006-01-02", *body.Date)
		if dateErr != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input.Date = &settledDate
	}
	row, err := c.settlements.Update(ctx, actor(ctx), tripContext(ctx), id, input)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "SETTLEMENT_UPDATED", settlementresponse.FromDomain(*row, tripContext(ctx)))
}

// approve godoc
// @Summary Approve a pending settlement
// @Tags settlements
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Success 200 {object} response.Envelope{data=settlementresponse.Settlement}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/settlements/{id}/approve [post]
func (c *controller) approve(ctx *gin.Context) {
	id, ok := settlementID(ctx)
	if !ok {
		return
	}
	row, err := c.settlements.Approve(ctx, actor(ctx), tripContext(ctx), id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "SETTLEMENT_APPROVED", settlementresponse.FromDomain(*row, tripContext(ctx)))
}

// reject godoc
// @Summary Reject a pending settlement
// @Tags settlements
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Param body body settlementrequest.Reject true "Rejection reason"
// @Success 200 {object} response.Envelope{data=settlementresponse.Settlement}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/settlements/{id}/reject [post]
func (c *controller) reject(ctx *gin.Context) {
	id, ok := settlementID(ctx)
	if !ok {
		return
	}
	var body settlementrequest.Reject
	if !bind(ctx, &body) {
		return
	}
	row, err := c.settlements.Reject(ctx, actor(ctx), tripContext(ctx), id, body.Reason)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "SETTLEMENT_REJECTED", settlementresponse.FromDomain(*row, tripContext(ctx)))
}

// delete godoc
// @Summary Delete a non-approved settlement
// @Tags settlements
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Settlement ID"
// @Success 200 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/settlements/{id} [delete]
func (c *controller) delete(ctx *gin.Context) {
	id, ok := settlementID(ctx)
	if !ok {
		return
	}
	if err := c.settlements.Delete(ctx, actor(ctx), tripContext(ctx), id); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "SETTLEMENT_DELETED")
}

// rateSet godoc
// @Summary Set a locked trip exchange rate
// @Tags finance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param body body fxrequest.SetRate true "Rate"
// @Success 200 {object} response.Envelope{data=fxresponse.Rate}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/exchange-rates [put]
func (c *controller) rateSet(ctx *gin.Context) {
	var body fxrequest.SetRate
	if !bind(ctx, &body) {
		return
	}
	rate, err := decimal.NewFromString(body.Rate)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	row, err := c.fx.SetTripRate(ctx, actor(ctx), tripContext(ctx), fxdomain.SetRateInput{FromCurrency: body.From, ToCurrency: body.To, Rate: rate})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "RATE_SET", fxresponse.FromDomain(*row))
}

// rateDelete godoc
// @Summary Delete a trip exchange rate
// @Tags finance
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param from query string true "From currency"
// @Param to query string true "To currency"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/exchange-rates [delete]
func (c *controller) rateDelete(ctx *gin.Context) {
	from, to := ctx.Query("from"), ctx.Query("to")
	if from == "" || to == "" {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	if err := c.fx.DeleteTripRate(ctx, actor(ctx), tripContext(ctx), from, to); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "RATE_DELETED")
}

// ratesTrip godoc
// @Summary List effective trip exchange rates
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope{data=[]fxresponse.Rate}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /trips/{code}/exchange-rates [get]
func (c *controller) ratesTrip(ctx *gin.Context) {
	rows, err := c.fx.ListForTrip(ctx, tripContext(ctx))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "RATES_FETCHED", fxresponse.ListFromDomain(rows))
}

// ratesGlobal godoc
// @Summary List global exchange rates
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=[]fxresponse.Rate}
// @Failure 401 {object} response.Envelope
// @Router /exchange-rates [get]
func (c *controller) ratesGlobal(ctx *gin.Context) {
	rows, err := c.fx.ListGlobal(ctx)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "RATES_FETCHED", fxresponse.ListFromDomain(rows))
}

// finalize godoc
// @Summary Finalize a trip and lock rates
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope{data=balanceresponse.FinalPlan}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/finalize [post]
func (c *controller) finalize(ctx *gin.Context) {
	plan, err := c.final.Finalize(ctx, actor(ctx), tripContext(ctx))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "TRIP_FINALIZED", balanceresponse.FinalPlanFromDomain(*plan))
}

// unfinalize godoc
// @Summary Reopen a finalized trip while retaining locked rates
// @Tags finance
// @Produce json
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/unfinalize [post]
func (c *controller) unfinalize(ctx *gin.Context) {
	if err := c.final.Unfinalize(ctx, actor(ctx), tripContext(ctx)); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "TRIP_UNFINALIZED")
}
