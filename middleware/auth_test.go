package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/middleware"
	"github.com/jungo-dev/junkit/response"
	"github.com/jungo-dev/junkit/security"
)

type authIdentity struct{ UserID int64 }

// fakeAuthenticator accepts "good" and returns the configured error for anything else.
func fakeAuthenticator(failErr error) middleware.Authenticator[*authIdentity] {
	return middleware.AuthenticatorFunc[*authIdentity](func(_ context.Context, token string) (*authIdentity, error) {
		if token == "good" {
			return &authIdentity{UserID: 7}, nil
		}
		return nil, failErr
	})
}

func newBearerRouter(a middleware.Authenticator[*authIdentity]) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", middleware.BearerAuth(a, newResponder(), middleware.BearerAuthOptions{}), func(c *gin.Context) {
		id := security.MustGetIdentity[*authIdentity](c)
		if fromReq, ok := security.GetIdentity[*authIdentity](c.Request.Context()); !ok || fromReq != id {
			c.Status(http.StatusTeapot)
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": id.UserID})
	})
	return r
}

func doBearer(r *gin.Engine, authHeader string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestBearerAuth_ValidToken(t *testing.T) {
	r := newBearerRouter(fakeAuthenticator(nil))
	for _, h := range []string{"Bearer good", "bearer good", "BEARER   good  "} {
		if w := doBearer(r, h); w.Code != http.StatusOK {
			t.Errorf("header %q: status = %d, want 200 (body %s)", h, w.Code, w.Body)
		}
	}
}

func TestBearerAuth_MissingOrMalformedHeader(t *testing.T) {
	r := newBearerRouter(fakeAuthenticator(nil))
	for _, h := range []string{"", "Bearer", "Bearer   ", "Basic good", "good", "Bearergood"} {
		if w := doBearer(r, h); w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", h, w.Code)
		}
	}
}

func TestBearerAuth_AppErrorPassesThrough(t *testing.T) {
	r := newBearerRouter(fakeAuthenticator(response.New(response.Unauthorized, "token_expired")))
	w := doBearer(r, "Bearer bad")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestBearerAuth_InfrastructureErrorIs500(t *testing.T) {
	r := newBearerRouter(fakeAuthenticator(errors.New("db down")))
	if w := doBearer(r, "Bearer bad"); w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func newSecretRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", middleware.RequireSecret("X-Internal-Secret", secret, newResponder()), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func doSecret(r *gin.Engine, value string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if value != "" {
		req.Header.Set("X-Internal-Secret", value)
	}
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRequireSecret(t *testing.T) {
	r := newSecretRouter("s3cret")
	if got := doSecret(r, "s3cret"); got != http.StatusOK {
		t.Errorf("correct secret: status = %d, want 200", got)
	}
	for _, v := range []string{"", "wrong", "s3cret "} {
		if got := doSecret(r, v); got != http.StatusForbidden {
			t.Errorf("secret %q: status = %d, want 403", v, got)
		}
	}
}

func TestRequireSecret_EmptySecretFailsClosed(t *testing.T) {
	r := newSecretRouter("")
	if got := doSecret(r, ""); got != http.StatusForbidden {
		t.Errorf("status = %d, want 403 when no secret is configured", got)
	}
}
