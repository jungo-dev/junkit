package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/middleware"
	"github.com/jungo-dev/junkit/tracer"
)

func newTracerDebugRouter(key, value string, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.TracerDebug(key, value))
	router.GET("/", handler)
	return router
}

func TestTracerDebug_PassesThroughWhenNotActivated(t *testing.T) {
	router := newTracerDebugRouter("t_debug", "1234", func(c *gin.Context) {
		c.String(http.StatusTeapot, "normal response")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d (the handler's own response, untouched)", w.Code, http.StatusTeapot)
	}
	if w.Body.String() != "normal response" {
		t.Fatalf("body = %q, want the handler's own response", w.Body.String())
	}
}

func TestTracerDebug_KeyOrValueEmptyDisables(t *testing.T) {
	router := newTracerDebugRouter("", "", func(c *gin.Context) {
		c.String(http.StatusOK, "normal")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?t_debug=anything", nil))

	if w.Body.String() != "normal" {
		t.Fatalf("body = %q, want the handler's own response when key/value are both empty", w.Body.String())
	}
}

func TestTracerDebug_RendersDashboardWhenActivated(t *testing.T) {
	router := newTracerDebugRouter("t_debug", "1234", func(c *gin.Context) {
		c.String(http.StatusTeapot, "normal response")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil))

	// The dashboard always renders 200 with its own body, regardless of
	// what the handler tried to write — that's the whole point of buffering.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for the debug dashboard", w.Code, http.StatusOK)
	}
	if strings.Contains(w.Body.String(), "normal response") {
		t.Fatal("the handler's real response body leaked through instead of being replaced by the dashboard")
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html for a browser client", ct)
	}
}

func TestTracerDebug_RendersJSONForCLIClients(t *testing.T) {
	router := newTracerDebugRouter("t_debug", "1234", func(c *gin.Context) {
		c.String(http.StatusOK, "normal response")
	})

	req := httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil)
	req.Header.Set("User-Agent", "curl/8.0.1")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Body.Len() == 0 {
		t.Fatal("expected a non-empty rendered body for a curl client")
	}
	// A curl client gets the JSON report (pipeable into jq), not HTML markup.
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json for a CLI client", ct)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Errorf("body is not valid JSON: %s", w.Body.String())
	}
}

func TestTracerDebug_BreakpointSignalRendersDashboardInstead(t *testing.T) {
	// BreakpointSignal should render the debug dashboard instead of propagating as a panic error.
	router := newTracerDebugRouter("t_debug", "1234", func(c *gin.Context) {
		panic(tracer.BreakpointSignal{})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — a BreakpointSignal should still render the dashboard", w.Code, http.StatusOK)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected a rendered dashboard body after a BreakpointSignal")
	}
}

func TestTracerDebug_OtherPanicsPropagate(t *testing.T) {
	// Non-breakpoint panics must propagate for Recover middleware to handle.
	router := newTracerDebugRouter("t_debug", "1234", func(c *gin.Context) {
		panic("a real error, not a breakpoint")
	})

	defer func() {
		r := recover()
		if r != "a real error, not a breakpoint" {
			t.Fatalf("recover() = %v, want the original panic value to propagate unchanged", r)
		}
	}()

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil))
}
