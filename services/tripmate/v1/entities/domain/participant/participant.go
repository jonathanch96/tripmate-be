package participant

import (
	"time"

	"github.com/google/uuid"
	domainuser "github.com/jblabs/tripmate-be/services/tripmate/v1/entities/domain/user"
)

type Role string

const (
	RolePlanner     Role = "planner"
	RoleParticipant Role = "participant"
)

type BankInfo struct{ BankName, AccountNumber, AccountHolder string }
type Participant struct {
	ID, TripID, UserID uuid.UUID
	Role               Role
	BankInfo           *BankInfo
	DisplayName        *string
	JoinedAt           time.Time
	User               *domainuser.PublicUser
}

// EffectiveName returns the per-trip nickname when set, falling back to the
// member's real account name or email — used to tell apart participants who
// share the same real name on a trip.
func (p Participant) EffectiveName() string {
	if p.DisplayName != nil && *p.DisplayName != "" {
		return *p.DisplayName
	}
	if p.User != nil {
		if p.User.Name != "" {
			return p.User.Name
		}
		return p.User.Email
	}
	return ""
}

func MaskAccount(value string) string {
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}
