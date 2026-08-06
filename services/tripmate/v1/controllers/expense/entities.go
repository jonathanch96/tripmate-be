package expense

import (
	expensedomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
)

type controller struct {
	trips    tripdomain.Service
	parts    participantdomain.Service
	expenses expensedomain.Service
}
