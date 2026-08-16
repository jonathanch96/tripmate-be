package invitation

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	invitationdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/invitation"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	invitationrequest "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/request/invitation"
	invitationresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/invitation"
	participantresponse "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/response/participant"
)

func NewController(trips tripdomain.Service, parts participantdomain.Service, invitations invitationdomain.Service) Controller {
	return &controller{trips: trips, parts: parts, invitations: invitations}
}

func (c *controller) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/invitations/me", c.listMine)
	group.POST("/invitations/:token/accept", c.accept)
	planner := group.Group("/trips/:code", middleware.RequirePlanner(c.trips, c.parts))
	planner.POST("/invitations", c.create)
	planner.GET("/invitations", c.listTrip)
	planner.DELETE("/invitations/:id", c.revoke)
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

// create godoc
// @Summary Invite by email
// @Tags invitations
// @Security BearerAuth
// @Param body body invitationrequest.Create true "Invitation"
// @Success 201 {object} response.Envelope{data=invitationresponse.InviteResult}
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/invitations [post]
func (c *controller) create(ctx *gin.Context) {
	var request invitationrequest.Create
	if !bind(ctx, &request) {
		return
	}
	result, err := c.invitations.Invite(ctx, actor(ctx).UserID, ctx.Param("code"), request.Email, request.Password)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	payload := invitationresponse.InviteResult{Status: result.Status}
	if result.Invitation != nil {
		invitation := invitationresponse.FromDomain(*result.Invitation)
		payload.Invitation = &invitation
		payload.InviteLink = "/invitations/" + result.Invitation.Token
	}
	if result.Participant != nil {
		participant := participantresponse.FromDomain(*result.Participant)
		payload.Participant = &participant
	}
	response.Created(ctx, "INVITATION_SENT", payload)
}

// listTrip godoc
// @Summary List trip invitations
// @Tags invitations
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /trips/{code}/invitations [get]
func (c *controller) listTrip(ctx *gin.Context) {
	rows, err := c.invitations.ListTrip(ctx, actor(ctx).UserID, ctx.Param("code"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "INVITATIONS_FETCHED", invitationresponse.FromDomains(rows))
}

// listMine godoc
// @Summary List my pending invitations
// @Tags invitations
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Router /invitations/me [get]
func (c *controller) listMine(ctx *gin.Context) {
	rows, err := c.invitations.ListForMe(ctx, actor(ctx).Email)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "INVITATIONS_FETCHED", invitationresponse.FromDomains(rows))
}

// accept godoc
// @Summary Accept an invitation
// @Tags invitations
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=participantresponse.Participant}
// @Failure 404 {object} response.Envelope
// @Router /invitations/{token}/accept [post]
func (c *controller) accept(ctx *gin.Context) {
	entity, err := c.invitations.Accept(ctx, actor(ctx).UserID, actor(ctx).Email, ctx.Param("token"))
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.OK(ctx, "INVITATION_ACCEPTED", participantresponse.FromDomain(*entity))
}

// revoke godoc
// @Summary Revoke an invitation
// @Tags invitations
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /trips/{code}/invitations/{id} [delete]
func (c *controller) revoke(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err == nil {
		err = c.invitations.Revoke(ctx, actor(ctx).UserID, ctx.Param("code"), id)
	}
	if err != nil {
		response.Error(ctx, err)
		return
	}
	response.NoData(ctx, "INVITATION_REVOKED")
}
