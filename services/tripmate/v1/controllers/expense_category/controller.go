package expense_category

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	categorydomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense_category"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	categoryrequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/expense_category"
	categoryresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/expense_category"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service, categories categorydomain.Service) Controller {
	return &controller{trips: trips, parts: parts, categories: categories}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	member := group.Group("/trips/:code/expense-categories", middleware.RequireTripMember(c.trips, c.parts))
	member.GET("", c.list)
	planner := group.Group("/trips/:code/expense-categories", middleware.RequirePlanner(c.trips, c.parts))
	planner.POST("", c.create)
	planner.DELETE("/:id", c.remove)
}

func bind(ctx *gin.Context, value any) bool {
	if err := ctx.ShouldBindJSON(value); err != nil {
		response.Error(ctx, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "request", Rule: "invalid", Message: err.Error()}}))
		return false
	}
	return true
}

// list godoc
// @Summary List expense categories for a trip
// @Tags expense-categories
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Success 200 {object} response.Envelope{data=[]expensecategoryresponse.ExpenseCategory}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/expense-categories [get]
func (c *controller) list(ctx *gin.Context) {
	tc := tripctx.MustFromContext(ctx.Request.Context())
	rows, err := c.categories.List(ctx, tc.Trip.ID)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "EXPENSE_CATEGORIES_FETCHED", categoryresponse.FromDomains(rows))
}

// create godoc
// @Summary Add a custom expense category to a trip
// @Tags expense-categories
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param body body expensecategoryrequest.Create true "Category"
// @Success 201 {object} response.Envelope{data=expensecategoryresponse.ExpenseCategory}
// @Failure 403 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /trips/{code}/expense-categories [post]
func (c *controller) create(ctx *gin.Context) {
	var req categoryrequest.Create
	if !bind(ctx, &req) {
		return
	}
	tc := tripctx.MustFromContext(ctx.Request.Context())
	entity, err := c.categories.Create(ctx, tc.Trip.ID, req.Name)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "EXPENSE_CATEGORY_CREATED", categoryresponse.FromDomain(*entity))
}

// remove godoc
// @Summary Remove a custom expense category from a trip
// @Tags expense-categories
// @Security BearerAuth
// @Param code path string true "Trip code"
// @Param id path string true "Category ID"
// @Success 200 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /trips/{code}/expense-categories/{id} [delete]
func (c *controller) remove(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("EXPENSE_CATEGORY_NOT_FOUND"))
		return
	}
	tc := tripctx.MustFromContext(ctx.Request.Context())
	if err := c.categories.Delete(ctx, tc.Trip.ID, id); err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "EXPENSE_CATEGORY_DELETED")
}
