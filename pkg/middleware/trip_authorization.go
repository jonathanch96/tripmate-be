package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
)

func RequireTripMember(trips tripdomain.Service, parts participantdomain.Service) gin.HandlerFunc {
	return tripGuard(trips, parts, false)
}
func RequirePlanner(trips tripdomain.Service, parts participantdomain.Service) gin.HandlerFunc {
	return tripGuard(trips, parts, true)
}
func tripGuard(trips tripdomain.Service, parts participantdomain.Service, planner bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		who := identity.MustFromContext(c.Request.Context())
		trip, err := trips.FindByCode(c.Request.Context(), c.Param("code"))
		if err != nil {
			c.Abort()
			response.Error(c, err)
			return
		}
		member, err := parts.GetMembership(c.Request.Context(), trip.ID, who.UserID)
		if err != nil {
			c.Abort()
			response.Error(c, apperror.New("NOT_TRIP_MEMBER"))
			return
		}
		if planner && member.Role != domainparticipant.RolePlanner {
			c.Abort()
			response.Error(c, apperror.New("PLANNER_ONLY"))
			return
		}
		v := tripctx.TripContext{Trip: *trip, Participant: *member}
		c.Request = c.Request.WithContext(tripctx.WithContext(c.Request.Context(), v))
		c.Next()
	}
}
