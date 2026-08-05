package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/jblabs/tripmate-be/pkg/response"
	"gorm.io/gorm"
)

func Health(c *gin.Context) {
	response.OK(c, "OK", gin.H{"status": "ok"})
}

func Ready(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := config.Ping(c.Request.Context(), db); err != nil {
			response.Error(c, apperror.Wrap(err, "INTERNAL_ERROR"))
			return
		}
		response.OK(c, "OK", gin.H{"status": "ready"})
	}
}
