package apperror

import "net/http"

type definition struct {
	HTTP    int
	Message string
}

var catalog = map[string]definition{
	"VALIDATION_FAILED":               {http.StatusBadRequest, "Request validation failed"},
	"INVALID_CURRENCY":                {http.StatusBadRequest, "Currency is not supported"},
	"SPLIT_SUM_MISMATCH":              {http.StatusBadRequest, "Splits must sum to the expense amount"},
	"PAYER_SUM_MISMATCH":              {http.StatusBadRequest, "Payers must sum to the expense amount"},
	"SETTLEMENT_EXCEEDS_DEBT":         {http.StatusBadRequest, "Settlement exceeds the outstanding debt"},
	"UNAUTHENTICATED":                 {http.StatusUnauthorized, "Authentication is required"},
	"INVALID_CREDENTIALS":             {http.StatusUnauthorized, "Email or password is incorrect"},
	"INVALID_CURRENT_PASSWORD":        {http.StatusUnauthorized, "Current password is incorrect"},
	"FORBIDDEN":                       {http.StatusForbidden, "You are not permitted to perform this action"},
	"NOT_TRIP_MEMBER":                 {http.StatusForbidden, "You are not a participant of this trip"},
	"PLANNER_ONLY":                    {http.StatusForbidden, "Only the trip planner can perform this action"},
	"EDIT_OWN_ONLY":                   {http.StatusForbidden, "You can only edit your own records"},
	"TRIP_FINALIZED":                  {http.StatusForbidden, "The trip has been finalized"},
	"TRIP_ARCHIVED":                   {http.StatusForbidden, "The trip has been archived"},
	"SETTLEMENT_NOT_ALLOWED_YET":      {http.StatusForbidden, "Settlements are not allowed before the trip ends"},
	"RECEIPT_IMAGE_LINK_INVALID":      {http.StatusForbidden, "This receipt image link is invalid or has expired"},
	"USER_NOT_FOUND":                  {http.StatusNotFound, "User not found"},
	"TRIP_NOT_FOUND":                  {http.StatusNotFound, "Trip not found"},
	"EXPENSE_NOT_FOUND":               {http.StatusNotFound, "Expense not found"},
	"SETTLEMENT_NOT_FOUND":            {http.StatusNotFound, "Settlement not found"},
	"PARTICIPANT_NOT_FOUND":           {http.StatusNotFound, "Participant not found"},
	"INVITATION_NOT_FOUND":            {http.StatusNotFound, "Invitation not found"},
	"RECEIPT_NOT_FOUND":               {http.StatusNotFound, "Receipt not found"},
	"EXPENSE_CATEGORY_NOT_FOUND":      {http.StatusNotFound, "Expense category not found"},
	"EMAIL_ALREADY_REGISTERED":        {http.StatusConflict, "Email is already registered"},
	"ALREADY_PARTICIPANT":             {http.StatusConflict, "User is already a trip participant"},
	"TRIP_CODE_COLLISION":             {http.StatusConflict, "Could not generate a unique trip code"},
	"CONCURRENT_MODIFICATION":         {http.StatusConflict, "The record was modified by another request"},
	"EXPENSE_CATEGORY_ALREADY_EXISTS": {http.StatusConflict, "A category with this name already exists"},
	"FILE_TOO_LARGE":                  {http.StatusRequestEntityTooLarge, "The uploaded file is too large"},
	"UNSUPPORTED_MEDIA_TYPE":          {http.StatusUnsupportedMediaType, "The uploaded file type is not supported"},
	"EXCHANGE_RATE_MISSING":           {http.StatusUnprocessableEntity, "An exchange rate is required"},
	"PARTICIPANT_HAS_ACTIVITY":        {http.StatusUnprocessableEntity, "Participant has financial activity"},
	"EXPENSE_CATEGORY_IS_DEFAULT":     {http.StatusUnprocessableEntity, "Default categories cannot be removed"},
	"SPLIT_PERCENT_MISMATCH":          {http.StatusBadRequest, "Split percentages must sum to 100"},
	"GOOGLE_TOKEN_INVALID":            {http.StatusUnauthorized, "Google sign-in token is invalid"},
	"RATE_LIMITED":                    {http.StatusTooManyRequests, "Too many requests"},
	"OCR_PROVIDER_ERROR":              {http.StatusBadGateway, "Receipt provider is unavailable"},
	"OCR_UNPARSEABLE":                 {http.StatusBadGateway, "Receipt provider returned an invalid response"},
	"OAUTH_INVALID_REQUEST":           {http.StatusBadRequest, "The OAuth request is invalid"},
	"OAUTH_INVALID_CLIENT":            {http.StatusUnauthorized, "Client authentication failed"},
	"OAUTH_INVALID_GRANT":             {http.StatusBadRequest, "The authorization grant is invalid"},
	"OAUTH_INVALID_SCOPE":             {http.StatusBadRequest, "The requested scope is invalid"},
	"OAUTH_INVALID_TARGET":            {http.StatusBadRequest, "The requested resource is invalid"},
	"OAUTH_UNSUPPORTED_GRANT_TYPE":    {http.StatusBadRequest, "The grant type is not supported"},
	"OAUTH_INVALID_REDIRECT_URI":      {http.StatusBadRequest, "The redirect URI is not allowed"},
	"OAUTH_INVALID_CLIENT_METADATA":   {http.StatusBadRequest, "The client metadata is invalid"},
	"OAUTH_INVALID_TOKEN":             {http.StatusUnauthorized, "The access token is invalid or expired"},
	"OAUTH_REQUEST_NOT_FOUND":         {http.StatusNotFound, "This authorization request was not found or was already answered"},
	"OAUTH_REQUEST_EXPIRED":           {http.StatusGone, "This authorization request has expired"},
	"OAUTH_GRANT_NOT_FOUND":           {http.StatusNotFound, "Connected app not found"},
	"INTERNAL_ERROR":                  {http.StatusInternalServerError, "An unexpected error occurred"},
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
