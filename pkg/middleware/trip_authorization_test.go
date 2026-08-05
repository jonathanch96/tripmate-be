package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/identity"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/tripctx"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
	domainparticipant "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/participant"
	domaintrip "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/trip"
)

type guardTripService struct {
	trip *domaintrip.Trip
	err  error
}

func (*guardTripService) Create(context.Context, uuid.UUID, tripdomain.CreateInput) (*domaintrip.Trip, error) {
	panic("unexpected Create")
}
func (*guardTripService) GetByCode(context.Context, uuid.UUID, string) (*domaintrip.Trip, error) {
	panic("unexpected GetByCode")
}
func (*guardTripService) ListMine(context.Context, uuid.UUID, tripdomain.ListFilter) ([]domaintrip.Trip, int64, error) {
	panic("unexpected ListMine")
}
func (*guardTripService) UpdateSettings(context.Context, uuid.UUID, string, tripdomain.UpdateSettingsInput) (*domaintrip.Trip, error) {
	panic("unexpected UpdateSettings")
}
func (s *guardTripService) FindByCode(context.Context, string) (*domaintrip.Trip, error) {
	return s.trip, s.err
}

type guardParticipantService struct {
	participant *domainparticipant.Participant
	err         error
}

func (*guardParticipantService) Join(context.Context, uuid.UUID, string) (*domainparticipant.Participant, error) {
	panic("unexpected Join")
}
func (*guardParticipantService) Add(context.Context, uuid.UUID, string, uuid.UUID) (*domainparticipant.Participant, error) {
	panic("unexpected Add")
}
func (*guardParticipantService) List(context.Context, uuid.UUID, string) ([]domainparticipant.Participant, error) {
	panic("unexpected List")
}
func (*guardParticipantService) Update(context.Context, uuid.UUID, string, uuid.UUID, *domainparticipant.BankInfo, *domainparticipant.Role) (*domainparticipant.Participant, error) {
	panic("unexpected Update")
}
func (*guardParticipantService) Remove(context.Context, uuid.UUID, string, uuid.UUID) error {
	panic("unexpected Remove")
}
func (s *guardParticipantService) GetMembership(context.Context, uuid.UUID, uuid.UUID) (*domainparticipant.Participant, error) {
	return s.participant, s.err
}

func TestTripAuthorizationMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	actor := uuid.New()
	trip := &domaintrip.Trip{ID: uuid.New(), Code: "ABC123"}
	cases := []struct {
		name       string
		planner    bool
		tripErr    error
		member     *domainparticipant.Participant
		memberErr  error
		wantStatus int
		wantCode   string
	}{
		{name: "missing trip", tripErr: apperror.New("TRIP_NOT_FOUND"), wantStatus: http.StatusNotFound, wantCode: "TRIP_NOT_FOUND"},
		{name: "non-member", memberErr: apperror.New("PARTICIPANT_NOT_FOUND"), wantStatus: http.StatusForbidden, wantCode: "NOT_TRIP_MEMBER"},
		{name: "member may read", member: &domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RoleParticipant}, wantStatus: http.StatusOK},
		{name: "participant may not plan", planner: true, member: &domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RoleParticipant}, wantStatus: http.StatusForbidden, wantCode: "PLANNER_ONLY"},
		{name: "planner may plan", planner: true, member: &domainparticipant.Participant{TripID: trip.ID, UserID: actor, Role: domainparticipant.RolePlanner}, wantStatus: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trips := &guardTripService{trip: trip, err: tc.tripErr}
			parts := &guardParticipantService{participant: tc.member, err: tc.memberErr}
			guard := RequireTripMember(trips, parts)
			if tc.planner {
				guard = RequirePlanner(trips, parts)
			}
			router := gin.New()
			router.GET("/trips/:code", func(c *gin.Context) {
				c.Request = c.Request.WithContext(identity.WithContext(c.Request.Context(), identity.Identity{UserID: actor}))
			}, guard, func(c *gin.Context) {
				value, ok := tripctx.FromContext(c.Request.Context())
				if !ok || value.Trip.Code != trip.Code || value.Participant.UserID != actor {
					t.Fatal("authorized request did not carry TripContext")
				}
				response.OK(c, "OK", nil)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trips/ABC123", nil))
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if tc.wantCode != "" {
				var envelope response.Envelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Code != tc.wantCode {
					t.Fatalf("code = %q, want %q", envelope.Code, tc.wantCode)
				}
			}
		})
	}
}
