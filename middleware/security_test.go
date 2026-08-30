package middleware_test

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/middleware"
)

func TestSecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("standard headers are always set", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		middleware.Security()(c)

		tests := map[string]string{
			"X-Content-Type-Options":       "nosniff",
			"X-Frame-Options":              "DENY",
			"Referrer-Policy":              "strict-origin-when-cross-origin",
			"Cross-Origin-Opener-Policy":   "same-origin-allow-popups",
			"Cross-Origin-Resource-Policy": "cross-origin",
			"Permissions-Policy":           "camera=(), microphone=(), geolocation=()",
		}
		for header, want := range tests {
			if got := w.Header().Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
	})

	t.Run("HSTS is only set over TLS", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		middleware.Security()(c)

		if got := w.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("Strict-Transport-Security = %q over plain HTTP, want empty", got)
		}
	})

	t.Run("HSTS is set when the request came over TLS", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.TLS = &tls.ConnectionState{}

		middleware.Security()(c)

		if got := w.Header().Get("Strict-Transport-Security"); got == "" {
			t.Error("Strict-Transport-Security should be set for a TLS request")
		}
	})
}
