package expense_category

import (
	categorydomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/expense_category"
	participantdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/participant"
	tripdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/trip"
)

type controller struct {
	trips      tripdomain.Service
	parts      participantdomain.Service
	categories categorydomain.Service
}
