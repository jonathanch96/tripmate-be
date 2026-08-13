package trip

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/money"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
	triprequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/trip"
	tripresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/trip"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service, balances BalanceService) Controller {
	return &controller{trips: trips, parts: parts, balances: balances}
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
		Name: request.Name, BaseCurrency: request.BaseCurrency, StartDate: startDate, EndDate: endDate,
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
	archived, _ := strconv.ParseBool(ctx.DefaultQuery("archived", "false"))
	actorID := actor(ctx).UserID
	rows, total, err := c.trips.ListMine(ctx, actorID, tripdomain.ListFilter{Page: page, PerPage: perPage, Archived: archived})
	if err != nil {
		response.Error(ctx, err)
		return
	}
	result := make([]tripresponse.Trip, len(rows))
	for index, entity := range rows {
		result[index] = tripresponse.FromDomain(entity, entity.PlannerID == actorID)
		result[index].Participants, result[index].NetBalance = c.summarize(ctx, actorID, entity)
	}
	response.List(ctx, "TRIPS_FETCHED", result, response.Pagination{Page: page, PerPage: perPage, TotalItems: total, TotalPages: (int(total) + perPage - 1) / perPage})
}

// summarize fetches the trip-card details the trips list needs beyond the trip row itself: the
// member avatars and the current user's net balance. Both are best-effort - a failure here
// shouldn't take down the whole trip list, so it's logged and the trip is returned without them.
func (c *controller) summarize(ctx *gin.Context, actorID uuid.UUID, entity domaintrip.Trip) ([]tripresponse.ParticipantSummary, *string) {
	members, err := c.parts.List(ctx, actorID, entity.Code)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "trip list: failed to load participants", "trip_code", entity.Code, "err", err)
		return nil, nil
	}
	summaries := make([]tripresponse.ParticipantSummary, 0, len(members))
	var actorParticipant *domainparticipant.Participant
	for i := range members {
		if members[i].User != nil {
			summaries = append(summaries, tripresponse.ParticipantSummary{ID: members[i].UserID, Name: members[i].User.Name, Email: members[i].User.Email})
		}
		if members[i].UserID == actorID {
			actorParticipant = &members[i]
		}
	}
	if actorParticipant == nil {
		return summaries, nil
	}
	result, err := c.balances.Calculate(ctx, tripctx.TripContext{Trip: entity, Participant: *actorParticipant})
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "trip list: failed to calculate balance", "trip_code", entity.Code, "err", err)
		return summaries, nil
	}
	scale := money.DisplayScale(entity.BaseCurrency)
	for _, balance := range result.Balances {
		if balance.UserID == actorID {
			netBalance := balance.NetBalance.StringFixedBank(scale)
			return summaries, &netBalance
		}
	}
	return summaries, nil
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
		Name: &request.Name, BaseCurrency: &request.BaseCurrency,
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
