package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/jungo-dev/junkit/logger"
	"github.com/jungo-dev/junkit/response"
)

// HeaderTraceID is the response (and, if present, request) header carrying the trace ID.
const HeaderTraceID = "X-Trace-Id"

// TraceOptions configures build and deployment metadata attached to requests.
type TraceOptions struct {
	// Version is the running build's version string, e.g. "v1.2.0".
	Version string
	// Commit is the running build's VCS commit hash.
	Commit string
	// Environment is the running environment, e.g. "production".
	Environment string
}

// Trace creates a Gin middleware that assigns or propagates a trace ID and sets request metadata.
//
// Usage:
//
//	router.Use(middleware.Trace(middleware.TraceOptions{Version: cfg.Version}))
func Trace(opts TraceOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		traceID := c.GetHeader(HeaderTraceID)
		if traceID == "" {
			traceID = uuid.NewString()
		}

		ctx := logger.WithTraceID(c.Request.Context(), traceID)
		c.Request = c.Request.WithContext(ctx)

		c.Writer.Header().Set(HeaderTraceID, traceID)

		c.Set(response.StartTimeContextKey, start)
		c.Set(response.TraceIDContextKey, traceID)
		c.Set(response.VersionContextKey, opts.Version)
		c.Set(response.EnvContextKey, opts.Environment)
		c.Set(commitContextKey, opts.Commit)

		c.Next()
	}
}

// commitContextKey stores the build commit hash in the Gin context.
const commitContextKey = "api_commit"
