package apperror

import "net/http"

type definition struct {
	HTTP    int
	Message string
}

var catalog = map[string]definition{
	"VALIDATION_FAILED":          {http.StatusBadRequest, "Request validation failed"},
	"INVALID_CURRENCY":           {http.StatusBadRequest, "Currency is not supported"},
	"SPLIT_SUM_MISMATCH":         {http.StatusBadRequest, "Splits must sum to the expense amount"},
	"PAYER_SUM_MISMATCH":         {http.StatusBadRequest, "Payers must sum to the expense amount"},
	"SETTLEMENT_EXCEEDS_DEBT":    {http.StatusBadRequest, "Settlement exceeds the outstanding debt"},
	"UNAUTHENTICATED":            {http.StatusUnauthorized, "Authentication is required"},
	"INVALID_CREDENTIALS":        {http.StatusUnauthorized, "Email or password is incorrect"},
	"FORBIDDEN":                  {http.StatusForbidden, "You are not permitted to perform this action"},
	"NOT_TRIP_MEMBER":            {http.StatusForbidden, "You are not a participant of this trip"},
	"PLANNER_ONLY":               {http.StatusForbidden, "Only the trip planner can perform this action"},
	"EDIT_OWN_ONLY":              {http.StatusForbidden, "You can only edit your own records"},
	"TRIP_FINALIZED":             {http.StatusForbidden, "The trip has been finalized"},
	"SETTLEMENT_NOT_ALLOWED_YET": {http.StatusForbidden, "Settlements are not allowed before the trip ends"},
	"USER_NOT_FOUND":             {http.StatusNotFound, "User not found"},
	"TRIP_NOT_FOUND":             {http.StatusNotFound, "Trip not found"},
	"EXPENSE_NOT_FOUND":          {http.StatusNotFound, "Expense not found"},
	"SETTLEMENT_NOT_FOUND":       {http.StatusNotFound, "Settlement not found"},
	"PARTICIPANT_NOT_FOUND":      {http.StatusNotFound, "Participant not found"},
	"INVITATION_NOT_FOUND":       {http.StatusNotFound, "Invitation not found"},
	"RECEIPT_NOT_FOUND":          {http.StatusNotFound, "Receipt not found"},
	"EMAIL_ALREADY_REGISTERED":   {http.StatusConflict, "Email is already registered"},
	"ALREADY_PARTICIPANT":        {http.StatusConflict, "User is already a trip participant"},
	"TRIP_CODE_COLLISION":        {http.StatusConflict, "Could not generate a unique trip code"},
	"CONCURRENT_MODIFICATION":    {http.StatusConflict, "The record was modified by another request"},
	"FILE_TOO_LARGE":             {http.StatusRequestEntityTooLarge, "The uploaded file is too large"},
	"UNSUPPORTED_MEDIA_TYPE":     {http.StatusUnsupportedMediaType, "The uploaded file type is not supported"},
	"EXCHANGE_RATE_MISSING":      {http.StatusUnprocessableEntity, "An exchange rate is required"},
	"PARTICIPANT_HAS_ACTIVITY":   {http.StatusUnprocessableEntity, "Participant has financial activity"},
	"RATE_LIMITED":               {http.StatusTooManyRequests, "Too many requests"},
	"OCR_PROVIDER_ERROR":         {http.StatusBadGateway, "Receipt provider is unavailable"},
	"OCR_UNPARSEABLE":            {http.StatusBadGateway, "Receipt provider returned an invalid response"},
	"INTERNAL_ERROR":             {http.StatusInternalServerError, "An unexpected error occurred"},
}

func Definition(code string) (httpStatus int, message string, ok bool) {
	d, ok := catalog[code]
	return d.HTTP, d.Message, ok
}

func Catalog() map[string]struct {
	HTTP    int
	Message string
} {
	result := make(map[string]struct {
		HTTP    int
		Message string
	}, len(catalog))
	for code, d := range catalog {
		result[code] = struct {
			HTTP    int
			Message string
		}{d.HTTP, d.Message}
	}
	return result
}
