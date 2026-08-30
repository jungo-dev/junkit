package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jungo-dev/junkit/middleware"
	"github.com/jungo-dev/junkit/response"
)

// newAccessLogRouter creates a test Gin router with AccessLog and observed logs.
func newAccessLogRouter(opts middleware.AccessLogOptions, handler gin.HandlerFunc) (*gin.Engine, *observer.ObservedLogs) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(response.TraceIDContextKey, "trace-123")
		c.Next()
	})
	router.Use(middleware.AccessLog(logger, opts))
	router.GET("/ping", handler)
	router.POST("/login", handler)
	router.GET("/health", handler)
	return router, logs
}

func TestAccessLog_LogsOneEntryPerRequest(t *testing.T) {
	router, logs := newAccessLogRouter(middleware.AccessLogOptions{}, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	all := logs.All()
	if len(all) != 1 {
		t.Fatalf("got %d log entries, want 1", len(all))
	}

	entry := all[0]
	if entry.Message != "HTTP_ACCESS_LOG" {
		t.Errorf("message = %q, want %q", entry.Message, "HTTP_ACCESS_LOG")
	}
	if entry.Level != zapcore.InfoLevel {
		t.Errorf("level = %v, want Info for a 2xx status", entry.Level)
	}

	fields := entry.ContextMap()
	if fields["status"] != int64(http.StatusOK) {
		t.Errorf("status field = %v, want %d", fields["status"], http.StatusOK)
	}
	if fields["method"] != http.MethodGet {
		t.Errorf("method field = %v, want %q", fields["method"], http.MethodGet)
	}
	if fields["path"] != "/ping" {
		t.Errorf("path field = %v, want %q", fields["path"], "/ping")
	}
	if fields["trace_id"] != "trace-123" {
		t.Errorf("trace_id field = %v, want %q", fields["trace_id"], "trace-123")
	}
}

func TestAccessLog_SeverityMatchesStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantLevel  zapcore.Level
	}{
		{name: "2xx is Info", statusCode: http.StatusOK, wantLevel: zapcore.InfoLevel},
		{name: "4xx is Warn", statusCode: http.StatusBadRequest, wantLevel: zapcore.WarnLevel},
		{name: "5xx is Error", statusCode: http.StatusInternalServerError, wantLevel: zapcore.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, logs := newAccessLogRouter(middleware.AccessLogOptions{}, func(c *gin.Context) {
				c.Status(tt.statusCode)
			})

			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

			all := logs.All()
			if len(all) != 1 {
				t.Fatalf("got %d log entries, want 1", len(all))
			}
			if all[0].Level != tt.wantLevel {
				t.Errorf("level = %v, want %v for status %d", all[0].Level, tt.wantLevel, tt.statusCode)
			}
		})
	}
}

func TestAccessLog_MasksSensitiveRequestFields(t *testing.T) {
	// Wire Payload before AccessLog to populate request payload for testing.
	router, logs := newAccessLogRouterWithPayload(t)

	body := `{"email":"a@b.com","password":"super-secret"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(httptest.NewRecorder(), req)

	all := logs.All()
	if len(all) != 1 {
		t.Fatalf("got %d log entries, want 1", len(all))
	}

	reqPayload, ok := all[0].ContextMap()["request_payload"].(map[string]any)
	if !ok {
		t.Fatalf("request_payload = %#v, want a map", all[0].ContextMap()["request_payload"])
	}
	if reqPayload["password"] != "[MASKED]" {
		t.Errorf("password field = %v, want it masked", reqPayload["password"])
	}
	if reqPayload["email"] != "a@b.com" {
		t.Errorf("email field = %v, want it untouched", reqPayload["email"])
	}
}

// newAccessLogRouterWithPayload wires Payload before AccessLog for payload testing.
func newAccessLogRouterWithPayload(t *testing.T) (*gin.Engine, *observer.ObservedLogs) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	router := gin.New()
	router.Use(middleware.Payload(middleware.PayloadOptions{MaxBodySize: 1 << 20}, newResponder()))
	router.Use(middleware.AccessLog(logger, middleware.AccessLogOptions{}))
	router.POST("/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return router, logs
}

func TestAccessLog_ExcludedPathIsNotLogged(t *testing.T) {
	router, logs := newAccessLogRouter(middleware.AccessLogOptions{}, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	if len(logs.All()) != 0 {
		t.Fatalf("got %d log entries for an excluded path, want 0", len(logs.All()))
	}
}

func TestAccessLog_SkipIfBypassesLogging(t *testing.T) {
	opts := middleware.AccessLogOptions{
		SkipIf: func(c *gin.Context) bool { return c.Query("t_debug") == "1234" },
	}
	router, logs := newAccessLogRouter(opts, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping?t_debug=1234", nil))

	if len(logs.All()) != 0 {
		t.Fatalf("got %d log entries, want 0 when SkipIf returns true", len(logs.All()))
	}
}

func TestAccessLog_CustomExcludedPathsOverrideDefaults(t *testing.T) {
	opts := middleware.AccessLogOptions{ExcludedPaths: []string{"/ping"}}
	router, logs := newAccessLogRouter(opts, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Verify /health is logged when default exclusions are overridden.
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if len(logs.All()) != 1 {
		t.Fatalf("got %d log entries for /health, want 1 since ExcludedPaths was overridden", len(logs.All()))
	}

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))
	if len(logs.All()) != 1 {
		t.Fatalf("got %d log entries total, want still 1 — /ping is excluded by the custom list", len(logs.All()))
	}
}
