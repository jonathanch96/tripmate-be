package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appLogger "github.com/jblabs/tripmate-be/pkg/logger"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/oklog/ulid/v2"
)

type CORSOptions struct {
	AllowedOrigins []string
	Production     bool
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if id == "" {
			id = ulid.Make().String()
		}
		c.Header("X-Request-ID", id)
		c.Request = c.Request.WithContext(appLogger.WithTrace(c.Request.Context(), id))
		c.Next()
	}
}

func Logging(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		ctx := c.Request.Context()
		log.InfoContext(ctx, "http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(started).Milliseconds(),
			"trace_id", appLogger.TraceID(ctx),
			"user_id", appLogger.UserID(ctx),
		)
	}
}

func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.ErrorContext(c.Request.Context(), "panic recovered", "panic", recovered, "stack", string(debug.Stack()))
				c.Abort()
				response.Error(c, apperror.New("INTERNAL_ERROR"))
			}
		}()
		c.Next()
	}
}

func CORS(options CORSOptions) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(options.AllowedOrigins))
	for _, origin := range options.AllowedOrigins {
		allowed[strings.TrimSpace(origin)] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		_, exact := allowed[origin]
		_, wildcard := allowed["*"]
		if origin != "" && (exact || (wildcard && !options.Production)) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if origin != "" && !exact && !(wildcard && !options.Production) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
