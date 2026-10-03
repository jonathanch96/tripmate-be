package mcp

import "github.com/gin-gonic/gin"

type Controller interface {
	RegisterProtocolRoutes(*gin.Engine)
}
