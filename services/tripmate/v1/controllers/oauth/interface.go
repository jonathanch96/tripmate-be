package oauth

import "github.com/gin-gonic/gin"

type Controller interface {
	RegisterRoutes(*gin.RouterGroup)
	RegisterProtocolRoutes(*gin.Engine)
}
