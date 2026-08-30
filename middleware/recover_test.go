package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/jungo-dev/junkit/i18n"
	"github.com/jungo-dev/junkit/middleware"
	"github.com/jungo-dev/junkit/notification"
	"github.com/jungo-dev/junkit/response"
	"github.com/jungo-dev/junkit/tracer"
)

func newResponder() response.Responder {
	return response.NewResponder(response.Options{Language: "en"}, i18n.NewTranslator())
}

// newRecoverRouter sets up a Gin router with Recover middleware for testing.
func newRecoverRouter(notifier notification.Notifier, opts middleware.RecoverOptions, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.Recover(zap.NewNop(), notifier, newResponder(), opts))
	router.GET("/", handler)
	return router
}

func TestRecover_NoPanicPassesThrough(t *testing.T) {
	notifier := newFakeNotifier()
	router := newRecoverRouter(notifier, middleware.RecoverOptions{}, func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRecover_PanicIsCaughtAndReported(t *testing.T) {
	notifier := newFakeNotifier()
	router := newRecoverRouter(notifier, middleware.RecoverOptions{}, func(c *gin.Context) {
		panic("something broke")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	notifications := notifier.errorNotifications()
	if len(notifications) != 1 {
		t.Fatalf("got %d error notifications, want 1", len(notifications))
	}
	if notifications[0].statusCode != http.StatusInternalServerError {
		t.Fatalf("notified statusCode = %d, want %d", notifications[0].statusCode, http.StatusInternalServerError)
	}
	if notifications[0].data["error"] != "something broke" {
		t.Fatalf(`notified error = %v, want "something broke"`, notifications[0].data["error"])
	}
}

func TestRecover_NilNotifierIsSafe(t *testing.T) {
	router := newRecoverRouter(nil, middleware.RecoverOptions{}, func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d even with a nil notifier", w.Code, http.StatusInternalServerError)
	}
}

func TestRecover_BreakpointSignalIsRePanicked(t *testing.T) {
	notifier := newFakeNotifier()
	router := newRecoverRouter(notifier, middleware.RecoverOptions{}, func(c *gin.Context) {
		panic(tracer.BreakpointSignal{})
	})

	defer func() {
		r := recover()
		if _, ok := r.(tracer.BreakpointSignal); !ok {
			t.Fatalf("recover() = %v (%T), want it to re-panic a tracer.BreakpointSignal untouched", r, r)
		}
		if len(notifier.errorNotifications()) != 0 {
			t.Error("a BreakpointSignal is not a real error and should not trigger a notification")
		}
	}()

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRecover_DebugBypassRoutesThroughTracer(t *testing.T) {
	notifier := newFakeNotifier()
	opts := middleware.RecoverOptions{DebugQueryKey: "t_debug", DebugQueryValue: "1234"}
	router := newRecoverRouter(notifier, opts, func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil))

	// Debug mode delegates to tracer.E without writing a response.
	if w.Code != http.StatusOK && w.Code != 0 {
		t.Fatalf("status = %d, want no response written on the debug bypass path", w.Code)
	}
	if len(notifier.errorNotifications()) != 0 {
		t.Error("the debug bypass path should not send an error notification")
	}
}

// TestRecover_EmptyDebugValueNeverBypasses tests that an empty DebugQueryValue never triggers debug bypass.
func TestRecover_EmptyDebugValueNeverBypasses(t *testing.T) {
	notifier := newFakeNotifier()
	opts := middleware.RecoverOptions{DebugQueryKey: "t_debug", DebugQueryValue: ""}
	router := newRecoverRouter(notifier, opts, func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil)) // no query string at all

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d — an empty DebugQueryValue must never bypass the normal error path",
			w.Code, http.StatusInternalServerError)
	}
	if len(notifier.errorNotifications()) != 1 {
		t.Fatalf("got %d error notifications, want 1 (the normal path should still notify)", len(notifier.errorNotifications()))
	}
}
