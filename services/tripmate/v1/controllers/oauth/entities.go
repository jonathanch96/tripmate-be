package oauth

import (
	"github.com/jblabs/tripmate-be/pkg/middleware"
	oauthdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/oauth"
)

type controller struct {
	service                 oauthdomain.Service
	registerIP, tokenClient *middleware.RateLimiter
}
