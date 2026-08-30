package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/logger"
	"github.com/jungo-dev/junkit/middleware"
	"github.com/jungo-dev/junkit/response"
)

func TestTrace_GeneratesATraceIDWhenNoneProvided(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	middleware.Trace(middleware.TraceOptions{Version: "v1.0.0"})(c)

	traceID := w.Header().Get(middleware.HeaderTraceID)
	if traceID == "" {
		t.Fatal("Trace() did not set a response trace ID header")
	}

	if got := logger.GetTraceID(c.Request.Context()); got != traceID {
		t.Fatalf("logger.GetTraceID(request context) = %q, want it to match the response header %q", got, traceID)
	}

	ctxVal, _ := c.Get(response.TraceIDContextKey)
	if ctxVal != traceID {
		t.Fatalf(`gin context %q = %v, want it to match %q`, response.TraceIDContextKey, ctxVal, traceID)
	}
}

func TestTrace_PropagatesAnIncomingTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set(middleware.HeaderTraceID, "incoming-trace-id")

	middleware.Trace(middleware.TraceOptions{})(c)

	if got := w.Header().Get(middleware.HeaderTraceID); got != "incoming-trace-id" {
		t.Fatalf("response trace ID = %q, want the incoming header value %q", got, "incoming-trace-id")
	}
	if got := logger.GetTraceID(c.Request.Context()); got != "incoming-trace-id" {
		t.Fatalf("logger.GetTraceID() = %q, want %q", got, "incoming-trace-id")
	}
}

func TestTrace_SetsBuildMetadataInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	middleware.Trace(middleware.TraceOptions{Version: "v1.2.3", Environment: "production"})(c)

	if v, _ := c.Get(response.VersionContextKey); v != "v1.2.3" {
		t.Fatalf("gin context %q = %v, want %q", response.VersionContextKey, v, "v1.2.3")
	}
	if v, _ := c.Get(response.EnvContextKey); v != "production" {
		t.Fatalf("gin context %q = %v, want %q", response.EnvContextKey, v, "production")
	}
}
