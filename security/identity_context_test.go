package security_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/security"
)

type testIdentity struct{ UserID int64 }

func newGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

func TestIdentity_SetAndGetFromGinAndRequestContext(t *testing.T) {
	c := newGinContext()
	want := &testIdentity{UserID: 42}

	security.SetIdentity(c, want)

	if got, ok := security.GetIdentity[*testIdentity](c); !ok || got != want {
		t.Errorf("from gin.Context: got %v, %v", got, ok)
	}
	if got, ok := security.GetIdentity[*testIdentity](c.Request.Context()); !ok || got != want {
		t.Errorf("from request context: got %v, %v", got, ok)
	}
}

func TestIdentity_WrongTypeOrMissing(t *testing.T) {
	c := newGinContext()
	if _, ok := security.GetIdentity[*testIdentity](c); ok {
		t.Error("GetIdentity on empty context reported ok")
	}

	security.SetIdentity(c, "a string identity")
	if _, ok := security.GetIdentity[*testIdentity](c); ok {
		t.Error("GetIdentity with wrong type reported ok")
	}
}

func TestIdentity_WithIdentity(t *testing.T) {
	ctx := security.WithIdentity(context.Background(), testIdentity{UserID: 7})
	got, ok := security.GetIdentity[testIdentity](ctx)
	if !ok || got.UserID != 7 {
		t.Errorf("got %v, %v", got, ok)
	}
}

func TestIdentity_StringKeyDoesNotCollide(t *testing.T) {
	c := newGinContext()
	c.Set("identityKey", &testIdentity{UserID: 1})
	if _, ok := security.GetIdentity[*testIdentity](c); ok {
		t.Error("identity readable through a string key set by another package")
	}
}

func TestMustGetIdentity_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustGetIdentity did not panic on missing identity")
		}
	}()
	security.MustGetIdentity[*testIdentity](newGinContext())
}
