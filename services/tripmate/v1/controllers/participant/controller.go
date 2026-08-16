package participant

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	participantrequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/participant"
	participantresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/participant"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service) Controller {
	return &controller{trips: trips, parts: parts}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/trips/:code/join", c.join)
	member := group.Group("/trips/:code", middleware.RequireTripMember(c.trips, c.parts))
	member.GET("/participants", c.list)
	member.PATCH("/participants/:id", c.update)
	planner := group.Group("/trips/:code", middleware.RequirePlanner(c.trips, c.parts))
	planner.POST("/participants", c.add)
	planner.DELETE("/participants/:id", c.remove)
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

// join godoc
// @Summary Join a trip by code
// @Tags participants
// @Security BearerAuth
// @Success 201 {object} response.Envelope{data=participantresponse.Participant}
// @Failure 409 {object} response.Envelope
// @Router /trips/{code}/join [post]
func (c *controller) join(ctx *gin.Context) {
	entity, err := c.parts.Join(ctx, actor(ctx).UserID, ctx.Param("code"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "TRIP_JOINED", participantresponse.FromDomain(*entity))
}

// list godoc
// @Summary List trip participants
// @Tags participants
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/participants [get]
func (c *controller) list(ctx *gin.Context) {
	rows, err := c.parts.List(ctx, actor(ctx).UserID, ctx.Param("code"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "PARTICIPANTS_FETCHED", participantresponse.FromDomains(rows))
}

// add godoc
// @Summary Add a participant
// @Tags participants
// @Security BearerAuth
// @Param body body participantrequest.Add true "Participant"
// @Success 201 {object} response.Envelope{data=participantresponse.Participant}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/participants [post]
func (c *controller) add(ctx *gin.Context) {
	var request participantrequest.Add
	if !bind(ctx, &request) {
		return
	}
	entity, err := c.parts.Add(ctx, actor(ctx).UserID, ctx.Param("code"), request.UserID)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.Created(ctx, "PARTICIPANT_ADDED", participantresponse.FromDomain(*entity))
}

// update godoc
// @Summary Update participant details
// @Tags participants
// @Security BearerAuth
// @Param body body participantrequest.Update true "Participant details"
// @Success 200 {object} response.Envelope{data=participantresponse.Participant}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/participants/{id} [patch]
func (c *controller) update(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		response.Error(ctx, apperror.New("PARTICIPANT_NOT_FOUND"))
		return
	}
	var request participantrequest.Update
	if !bind(ctx, &request) {
		return
	}
	var bank *domainparticipant.BankInfo
	if request.BankInfo != nil {
		bank = &domainparticipant.BankInfo{
			BankName: request.BankInfo.BankName, AccountNumber: request.BankInfo.AccountNumber,
			AccountHolder: request.BankInfo.AccountHolder,
		}
	}
	var role *domainparticipant.Role
	if request.Role != nil {
		value := domainparticipant.Role(*request.Role)
		role = &value
	}
	entity, err := c.parts.Update(ctx, actor(ctx).UserID, ctx.Param("code"), id, bank, role, request.DisplayName)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "PARTICIPANT_UPDATED", participantresponse.FromDomain(*entity))
}

// remove godoc
// @Summary Remove a participant
// @Tags participants
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/participants/{id} [delete]
func (c *controller) remove(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err == nil {
		err = c.parts.Remove(ctx, actor(ctx).UserID, ctx.Param("code"), id)
	}
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "PARTICIPANT_REMOVED")
}
