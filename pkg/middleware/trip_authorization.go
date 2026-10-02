package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		v, err := ResolveTrip(c.Request.Context(), trips, parts, c.Param("code"), who.UserID, planner)
		if err != nil {
			c.Abort()
			response.Error(c, err)
			return
		}
		c.Request = c.Request.WithContext(tripctx.WithContext(c.Request.Context(), *v))
		c.Next()
	}
}

// ResolveTrip loads a trip by code and the caller's membership of it, refusing non-members (and
// non-planners when planner is set). It is the one place trip access is decided, shared by the
// REST guards above and the MCP tools.
func ResolveTrip(ctx context.Context, trips tripdomain.Service, parts participantdomain.Service, code string, userID uuid.UUID, planner bool) (*tripctx.TripContext, error) {
	trip, err := trips.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	member, err := parts.GetMembership(ctx, trip.ID, userID)
	if err != nil {
		return nil, apperror.New("NOT_TRIP_MEMBER")
	}
	if planner && member.Role != domainparticipant.RolePlanner {
		return nil, apperror.New("PLANNER_ONLY")
	}
	return &tripctx.TripContext{Trip: *trip, Participant: *member}, nil
}
