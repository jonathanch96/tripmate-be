package tripctx

import (
	"context"

	"github.com/google/uuid"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type TripContext struct {
	Trip        domaintrip.Trip
	Participant domainparticipant.Participant
}
type key struct{}

func WithContext(ctx context.Context, v TripContext) context.Context {
	return context.WithValue(ctx, key{}, v)
}
func FromContext(ctx context.Context) (TripContext, bool) {
	v, ok := ctx.Value(key{}).(TripContext)
	return v, ok
}

func ForActor(ctx context.Context, actor uuid.UUID, code string) (TripContext, bool) {
	v, ok := FromContext(ctx)
	if !ok || v.Trip.Code != code || v.Participant.UserID != actor || v.Participant.TripID != v.Trip.ID {
		return TripContext{}, false
	}
	return v, true
}
func MustFromContext(ctx context.Context) TripContext {
	v, ok := FromContext(ctx)
	if !ok {
		panic("trip context is missing")
	}
	return v
}
