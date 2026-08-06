package invitation

import (
	invitationdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/invitation"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
)

type controller struct {
	trips       tripdomain.Service
	parts       participantdomain.Service
	invitations invitationdomain.Service
}
