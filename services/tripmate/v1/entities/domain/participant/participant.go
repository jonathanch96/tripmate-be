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
	JoinedAt           time.Time
	User               *domainuser.PublicUser
}

func MaskAccount(value string) string {
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}
