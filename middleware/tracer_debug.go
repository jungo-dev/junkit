package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/tracer"
)

// bodyWriter discards response output when debug mode replaces it with the tracer dashboard.
type bodyWriter struct {
	gin.ResponseWriter
}

// Write implements io.Writer by discarding b without copying it anywhere.
func (w *bodyWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

// TracerDebug renders an interactive debug dashboard when matching the debug query key and value.
//
// Usage:
//
//	router.Use(middleware.TracerDebug(cfg.TracerDebugKey, cfg.TracerDebugValue))
func TracerDebug(key, value string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if key == "" || value == "" || c.Query(key) != value {
			c.Next()
			return
		}

		start := time.Now()
		debugger := tracer.New()
		debugger.Enable()

		ctx := tracer.WithContext(c.Request.Context(), debugger)
		c.Request = c.Request.WithContext(ctx)

		bw := &bodyWriter{ResponseWriter: c.Writer}
		c.Writer = bw

		defer renderTracerDashboard(c, bw, start)

		c.Next()
	}
}

// renderTracerDashboard renders HTML or ANSI text debug dashboard for the request.
func renderTracerDashboard(c *gin.Context, bw *bodyWriter, start time.Time) {
	if r := recover(); r != nil {
		if _, ok := r.(tracer.BreakpointSignal); !ok {
			panic(r)
		}
		tracer.C(c.Request.Context(), fmt.Sprintf("⏸ Stopped by tracer.Stop() after %v", time.Since(start)))
	} else {
		tracer.C(c.Request.Context(), fmt.Sprintf("Request processed in %v", time.Since(start)))
		tracer.HTTPInfo(c.Request.Context(), c.Request)
	}

	c.Writer = bw.ResponseWriter
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	if isTerminalClient(c) {
		c.String(http.StatusOK, tracer.RenderText(c.Request.Context()))
	} else {
		c.String(http.StatusOK, tracer.RenderHTML(c.Request.Context()))
	}
}

// isTerminalClient reports whether the request's User-Agent identifies a CLI HTTP client.
func isTerminalClient(c *gin.Context) bool {
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	return strings.Contains(ua, "curl") || strings.Contains(ua, "httpie") || strings.Contains(ua, "wget")
}
