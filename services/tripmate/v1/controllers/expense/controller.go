package expense

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
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainexpense "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/expense"
	expenserequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/expense"
	expenseresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/expense"
	"github.com/shopspring/decimal"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service, expenses expensedomain.Service) Controller {
	return &controller{trips: trips, parts: parts, expenses: expenses}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	member := group.Group("/trips/:code/expenses", middleware.RequireTripMember(c.trips, c.parts))
	member.GET("", c.list)
	member.POST("", c.create)
	member.GET("/:id", c.get)
	member.PATCH("/:id", c.update)
	member.DELETE("/:id", c.delete)
	planner := group.Group("/trips/:code/expenses", middleware.RequirePlanner(c.trips, c.parts))
	planner.POST("/:id/approve", c.approve)
	planner.POST("/:id/reject", c.reject)
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

func parseRows(payers []expenserequest.Payer, splits []expenserequest.Split, participants []string) ([]domainexpense.Payer, map[uuid.UUID]decimal.Decimal, []uuid.UUID, error) {
	var resultPayers []domainexpense.Payer
	if payers != nil {
		resultPayers = make([]domainexpense.Payer, len(payers))
	}
	for i, row := range payers {
		id, err := uuid.Parse(row.UserID)
		if err != nil {
			return nil, nil, nil, apperror.New("VALIDATION_FAILED")
		}
		amount, err := decimal.NewFromString(row.Amount)
		if err != nil {
			return nil, nil, nil, apperror.New("VALIDATION_FAILED")
		}
		resultPayers[i] = domainexpense.Payer{UserID: id, Amount: amount}
	}
	var manual map[uuid.UUID]decimal.Decimal
	if splits != nil {
		manual = make(map[uuid.UUID]decimal.Decimal, len(splits))
	}
	for _, row := range splits {
		id, err := uuid.Parse(row.UserID)
		if err != nil {
			return nil, nil, nil, apperror.New("VALIDATION_FAILED")
		}
		amount, err := decimal.NewFromString(row.Amount)
		if err != nil {
			return nil, nil, nil, apperror.New("VALIDATION_FAILED")
		}
		if _, duplicate := manual[id]; duplicate {
			return nil, nil, nil, apperror.New("VALIDATION_FAILED")
		}
		manual[id] = amount
	}
	var ids []uuid.UUID
	if participants != nil {
		ids = make([]uuid.UUID, len(participants))
	}
	for i, raw := range participants {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, nil, nil, apperror.New("VALIDATION_FAILED")
		}
		ids[i] = id
	}
	return resultPayers, manual, ids, nil
}

func parseID(ctx *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("EXPENSE_NOT_FOUND"))
		return uuid.Nil, false
	}
	return id, true
}

// create godoc
// @Summary Create an expense
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param body body expenserequest.Create true "Expense"
// @Success 201 {object} response.Envelope{data=expenseresponse.Expense}
// @Failure 400 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expenses [post]
func (c *controller) create(ctx *gin.Context) {
	var req expenserequest.Create
	if !bind(ctx, &req) {
		return
	}
	date, err := time.Parse("2006-01-02", req.ExpenseDate)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		response.Error(ctx, apperror.New("VALIDATION_FAILED"))
		return
	}
	payers, manual, participants, err := parseRows(req.Payers, req.Splits, req.Participants)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	tc := tripContext(ctx)
	entity, err := c.expenses.Create(ctx, actor(ctx), tc, expensedomain.CreateInput{ExpenseDate: date, Description: req.Description, Amount: amount, Currency: req.Currency, SplitType: domainexpense.SplitType(req.SplitType), Payers: payers, Participants: participants, Manual: manual, Note: req.Note})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "EXPENSE_CREATED", expenseresponse.FromDomain(*entity, tc, actor(ctx).UserID))
}

// get godoc
// @Summary Get an expense
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Expense ID"
// @Success 200 {object} response.Envelope{data=expenseresponse.Expense}
// @Failure 404 {object} response.Envelope
// @Router /trips/{code}/expenses/{id} [get]
func (c *controller) get(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	tc := tripContext(ctx)
	entity, err := c.expenses.Get(ctx, tc, id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "EXPENSE_FETCHED", expenseresponse.FromDomain(*entity, tc, actor(ctx).UserID))
}

// update godoc
// @Summary Update an expense
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Expense ID"
// @Param body body expenserequest.Update true "Expense changes"
// @Success 200 {object} response.Envelope{data=expenseresponse.Expense}
// @Failure 400 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expenses/{id} [patch]
func (c *controller) update(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	var req expenserequest.Update
	if !bind(ctx, &req) {
		return
	}
	payers, manual, participants, err := parseRows(req.Payers, req.Splits, req.Participants)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	input := expensedomain.UpdateInput{Description: req.Description, Currency: req.Currency, Payers: payers, Participants: participants, Manual: manual, Note: req.Note, Version: req.Version}
	if req.ExpenseDate != nil {
		value, parseErr := time.Parse("2006-01-02", *req.ExpenseDate)
		if parseErr != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input.ExpenseDate = &value
	}
	if req.Amount != nil {
		value, parseErr := decimal.NewFromString(*req.Amount)
		if parseErr != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		input.Amount = &value
	}
	if req.SplitType != nil {
		value := domainexpense.SplitType(*req.SplitType)
		input.SplitType = &value
	}
	tc := tripContext(ctx)
	entity, err := c.expenses.Update(ctx, actor(ctx), tc, id, input)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "EXPENSE_UPDATED", expenseresponse.FromDomain(*entity, tc, actor(ctx).UserID))
}

// delete godoc
// @Summary Delete an expense
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Expense ID"
// @Success 200 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expenses/{id} [delete]
func (c *controller) delete(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	if err := c.expenses.Delete(ctx, actor(ctx), tripContext(ctx), id); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "EXPENSE_DELETED")
}

// approve godoc
// @Summary Approve a pending expense
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Expense ID"
// @Success 200 {object} response.Envelope{data=expenseresponse.Expense}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expenses/{id}/approve [post]
func (c *controller) approve(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	tc := tripContext(ctx)
	entity, err := c.expenses.Approve(ctx, actor(ctx), tc, id)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "EXPENSE_APPROVED", expenseresponse.FromDomain(*entity, tc, actor(ctx).UserID))
}

// reject godoc
// @Summary Reject a pending expense
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Expense ID"
// @Param body body expenserequest.Reject true "Rejection reason"
// @Success 200 {object} response.Envelope{data=expenseresponse.Expense}
// @Failure 400 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expenses/{id}/reject [post]
func (c *controller) reject(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}
	var req expenserequest.Reject
	if !bind(ctx, &req) {
		return
	}
	tc := tripContext(ctx)
	entity, err := c.expenses.Reject(ctx, actor(ctx), tc, id, req.Reason)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "EXPENSE_REJECTED", expenseresponse.FromDomain(*entity, tc, actor(ctx).UserID))
}

// list godoc
// @Summary List trip expenses
// @Tags expenses
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param status query string false "pending, approved, or rejected"
// @Param currency query string false "ISO currency"
// @Param payer_user_id query string false "Payer user ID"
// @Param split_user_id query string false "Split participant user ID"
// @Param date_from query string false "Start date (YYYY-MM-DD)"
// @Param date_to query string false "End date (YYYY-MM-DD)"
// @Param q query string false "Description search"
// @Param sort query string false "date_desc, amount_desc, or created_desc"
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Success 200 {object} response.Envelope{data=[]expenseresponse.Expense}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expenses [get]
func (c *controller) list(ctx *gin.Context) {
	filter := expensedomain.Filter{Currency: ctx.Query("currency"), Query: ctx.Query("q"), Sort: ctx.DefaultQuery("sort", "date_desc")}
	filter.Page, _ = strconv.Atoi(ctx.DefaultQuery("page", "1"))
	filter.PerPage, _ = strconv.Atoi(ctx.DefaultQuery("per_page", "20"))
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 || filter.PerPage > 100 {
		filter.PerPage = 20
	}
	if raw := ctx.Query("payer_user_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		filter.PayerUserID = &id
	}
	if raw := ctx.Query("split_user_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		filter.SplitUserID = &id
	}
	if raw := ctx.Query("status"); raw != "" {
		value := domainexpense.Status(raw)
		filter.Status = &value
	}
	if raw := ctx.Query("date_from"); raw != "" {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		filter.DateFrom = &value
	}
	if raw := ctx.Query("date_to"); raw != "" {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			response.Error(ctx, apperror.New("VALIDATION_FAILED"))
			return
		}
		filter.DateTo = &value
	}
	tc := tripContext(ctx)
	rows, total, totals, err := c.expenses.List(ctx, tc, filter)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	result := make([]expenseresponse.Expense, len(rows))
	for i := range rows {
		result[i] = expenseresponse.FromDomain(rows[i], tc, actor(ctx).UserID)
	}
	byCurrency := make(map[string]string, len(totals.ByCurrency))
	for currency, amount := range totals.ByCurrency {
		byCurrency[currency] = amount.StringFixedBank(displayScale(currency))
	}
	countByStatus := make(map[string]int64, len(totals.CountByStatus))
	for status, count := range totals.CountByStatus {
		countByStatus[string(status)] = count
	}
	response.ListWithMeta(ctx, "EXPENSES_FETCHED", result, response.Pagination{Page: filter.Page, PerPage: filter.PerPage, TotalItems: total, TotalPages: (int(total) + filter.PerPage - 1) / filter.PerPage}, gin.H{"totals": gin.H{"by_currency": byCurrency, "count_by_status": countByStatus}})
}

func displayScale(currency string) int32 {
	switch currency {
	case "IDR", "JPY", "KRW", "VND":
		return 0
	default:
		return 2
	}
}
