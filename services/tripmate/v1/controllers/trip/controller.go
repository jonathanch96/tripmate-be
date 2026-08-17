package trip

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	triprequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/trip"
	tripresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/trip"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service) Controller {
	return &controller{trips: trips, parts: parts}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/trips", c.create)
	group.GET("/trips", c.list)
	member := group.Group("/trips/:code", middleware.RequireTripMember(c.trips, c.parts))
	member.GET("", c.get)
	planner := group.Group("/trips/:code", middleware.RequirePlanner(c.trips, c.parts))
	planner.PATCH("", c.update)
	planner.POST("/archive", c.archive)
	planner.POST("/unarchive", c.unarchive)
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

func createSettings(request triprequest.Create) domaintrip.Settings {
	return domaintrip.Settings{
		EditPermission:              domaintrip.EditPermission(request.EditPermission),
		ApprovalRequiredExpenses:    request.ApprovalRequiredExpenses,
		ApprovalRequiredSettlements: request.ApprovalRequiredSettlements,
		MultiCurrencyEnabled:        request.MultiCurrencyEnabled,
		AllowSettlementBeforeEnd:    request.AllowSettlementBeforeEnd,
	}
}

func updateSettings(request triprequest.Update) domaintrip.Settings {
	return domaintrip.Settings{
		EditPermission:              domaintrip.EditPermission(request.EditPermission),
		ApprovalRequiredExpenses:    request.ApprovalRequiredExpenses,
		ApprovalRequiredSettlements: request.ApprovalRequiredSettlements,
		MultiCurrencyEnabled:        request.MultiCurrencyEnabled,
		AllowSettlementBeforeEnd:    request.AllowSettlementBeforeEnd,
	}
}

// create godoc
// @Summary Create a trip
// @Tags trips
// @Security BearerAuth
// @Param body body triprequest.Create true "Trip"
// @Success 201 {object} response.Envelope{data=tripresponse.Trip}
// @Failure 400 {object} response.Envelope
// @Router /trips [post]
func (c *controller) create(ctx *gin.Context) {
	var request triprequest.Create
	if !bind(ctx, &request) {
		return
	}
	startDate, startErr := time.Parse("2006-01-02", request.StartDate)
	endDate, endErr := time.Parse("2006-01-02", request.EndDate)
	if startErr != nil || endErr != nil {
		response.Error(ctx, apperror.WithFields("VALIDATION_FAILED", []apperror.FieldError{{Field: "start_date", Rule: "datetime", Message: "dates must use YYYY-MM-DD"}}))
		return
	}
	entity, err := c.trips.Create(ctx, actor(ctx).UserID, tripdomain.CreateInput{
		Name: request.Name, BaseCurrency: request.BaseCurrency, Country: request.Country, StartDate: startDate, EndDate: endDate,
		Settings: createSettings(request),
	})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "TRIP_CREATED", tripresponse.FromDomain(*entity, true))
}

// list godoc
// @Summary List my trips
// @Tags trips
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Router /trips [get]
func (c *controller) list(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(ctx.DefaultQuery("per_page", "20"))
	filter := tripdomain.ListFilter{Page: page, PerPage: perPage}
	if raw := ctx.Query("archived"); raw != "" {
		value := raw == "true"
		filter.Archived = &value
	}
	rows, total, err := c.trips.ListMine(ctx, actor(ctx).UserID, filter)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	result := make([]tripresponse.Trip, len(rows))
	for index, entity := range rows {
		result[index] = tripresponse.FromDomain(entity, entity.PlannerID == actor(ctx).UserID)
	}
	response.List(ctx, "TRIPS_FETCHED", result, response.Pagination{Page: page, PerPage: perPage, TotalItems: total, TotalPages: (int(total) + perPage - 1) / perPage})
}

// get godoc
// @Summary Get a trip
// @Tags trips
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=tripresponse.Trip}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code} [get]
func (c *controller) get(ctx *gin.Context) {
	entity, err := c.trips.GetByCode(ctx, actor(ctx).UserID, ctx.Param("code"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "TRIP_FETCHED", tripresponse.FromDomain(*entity, entity.PlannerID == actor(ctx).UserID))
}

// update godoc
// @Summary Update trip settings
// @Tags trips
// @Security BearerAuth
// @Param body body triprequest.Update true "Trip settings"
// @Success 200 {object} response.Envelope{data=tripresponse.Trip}
// @Failure 400 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /trips/{code} [patch]
func (c *controller) update(ctx *gin.Context) {
	var request triprequest.Update
	if !bind(ctx, &request) {
		return
	}
	entity, err := c.trips.UpdateSettings(ctx, actor(ctx).UserID, ctx.Param("code"), tripdomain.UpdateSettingsInput{
		Name: &request.Name, BaseCurrency: &request.BaseCurrency, Country: request.Country,
		Settings: updateSettings(request), Version: request.Version,
	})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "TRIP_UPDATED", tripresponse.FromDomain(*entity, true))
}

// archive godoc
// @Summary Archive a trip
// @Tags trips
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=tripresponse.Trip}
// @Failure 400 {object} response.Envelope
// @Router /trips/{code}/archive [post]
func (c *controller) archive(ctx *gin.Context) {
	entity, err := c.trips.Archive(ctx, actor(ctx).UserID, ctx.Param("code"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "TRIP_ARCHIVED", tripresponse.FromDomain(*entity, true))
}

// unarchive godoc
// @Summary Restore an archived trip
// @Tags trips
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=tripresponse.Trip}
// @Failure 400 {object} response.Envelope
// @Router /trips/{code}/unarchive [post]
func (c *controller) unarchive(ctx *gin.Context) {
	entity, err := c.trips.Unarchive(ctx, actor(ctx).UserID, ctx.Param("code"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "TRIP_UNARCHIVED", tripresponse.FromDomain(*entity, true))
}
