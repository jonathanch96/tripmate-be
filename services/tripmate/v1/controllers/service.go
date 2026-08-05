package controllers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/pkg/response"
	"gorm.io/gorm"
)

type Dependencies struct {
	DB  *gorm.DB
	Cfg *config.Config
	Log *slog.Logger
}

type Service struct{}

func NewService(_ Dependencies) *Service { return &Service{} }

func (s *Service) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/ping", s.Ping)
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
