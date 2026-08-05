package controllers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	apphash "github.com/jblabs/tripmate-be/pkg/hash"
	appjwt "github.com/jblabs/tripmate-be/pkg/jwt"
	"github.com/jblabs/tripmate-be/pkg/middleware"
	"github.com/jblabs/tripmate-be/pkg/response"
	authcontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/auth"
	usercontroller "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers/user"
	refreshtokens "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/refresh_tokens"
	users "github.com/jblabs/tripmate-be/services/tripmate/v1/db/tripmate/users"
	userdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/user"
	"gorm.io/gorm"
)

type Dependencies struct {
	DB  *gorm.DB
	Cfg *config.Config
	Log *slog.Logger
}

type Service struct {
	auth   authcontroller.Controller
	users  usercontroller.Controller
	issuer *appjwt.Issuer
}

func NewService(deps Dependencies) *Service {
	issuer := appjwt.NewIssuer(appjwt.Config{
		AccessSecret: deps.Cfg.JWT.AccessSecret, RefreshSecret: deps.Cfg.JWT.RefreshSecret,
		AccessTTL: deps.Cfg.JWT.AccessTTL, RefreshTTL: deps.Cfg.JWT.RefreshTTL,
	})
	userService := userdomain.NewService(userdomain.Dependencies{
		Repo:   users.NewGormPostgresqlAdapter(deps.DB),
		Tokens: refreshtokens.NewGormPostgresqlAdapter(deps.DB),
		Hasher: apphash.NewArgon2Hasher(), Issuer: issuer,
	})
	return &Service{auth: authcontroller.NewController(userService),
		users: usercontroller.NewController(userService), issuer: issuer}
}

func (s *Service) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/ping", s.Ping)
	s.auth.RegisterRoutes(group)
	protected := group.Group("")
	protected.Use(middleware.Authenticate(s.issuer))
	s.users.RegisterRoutes(protected)
}

// Ping godoc
// @Summary Check API connectivity
// @Description Returns a fully populated envelope proving the API composition root is wired.
// @Tags system
// @Produce json
// @Success 200 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /ping [get]
func (s *Service) Ping(c *gin.Context) {
	response.OK(c, "PONG", gin.H{"pong": true})
}
