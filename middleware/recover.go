package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/jungo-dev/junkit/notification"
	"github.com/jungo-dev/junkit/response"
	"github.com/jungo-dev/junkit/tracer"
)

// RecoverOptions configures panic recovery behavior and debug bypass settings.
type RecoverOptions struct {
	DebugQueryKey   string
	DebugQueryValue string
}

// Recover creates a Gin middleware that catches panics, logs stack traces, sends error notifications, and returns HTTP 500.
//
// Usage:
//
//	router.Use(middleware.Recover(zapLogger, notifier, responder, middleware.RecoverOptions{}))
func Recover(logger *zap.Logger, notifier notification.Notifier, responder response.Responder, opts RecoverOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}

			if _, ok := r.(tracer.BreakpointSignal); ok {
				panic(r)
			}

			if opts.DebugQueryKey != "" && opts.DebugQueryValue != "" && c.Query(opts.DebugQueryKey) == opts.DebugQueryValue {
				tracer.E(c.Request.Context(), "PANIC RECOVERED", r)
				return
			}

			handlePanic(c, logger, notifier, responder, r)
		}()

		c.Next()
	}
}

// handlePanic handles logging, notification, and 500 response generation for recovered panics.
func handlePanic(c *gin.Context, logger *zap.Logger, notifier notification.Notifier, responder response.Responder, r any) {
	file, line, fn := tracer.FindErrorOrigin()
	stack := debug.Stack()

	logger.Error("panic recovered",
		zap.Any("error", r),
		zap.String("location", file),
		zap.Int("line", line),
		zap.String("function", fn),
		zap.ByteString("stack", stack),
	)

	errorData := map[string]any{
		"error":     fmt.Sprintf("%v", r),
		"location":  file,
		"line":      line,
		"function":  fn,
		"stack":     string(stack),
		"timestamp": time.Now().Format(time.RFC3339Nano),
	}
	if notifier != nil {
		notifier.SendErrorNotification(c, errorData, http.StatusInternalServerError)
	}

	responder.Send(c, http.StatusInternalServerError, "internal_server_error")
	c.Abort()
}
