package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/i18n"
	"github.com/jungo-dev/junkit/middleware"
	"github.com/jungo-dev/junkit/response"
)

func newPayloadContext(body string, contentType string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return w, c
}

func TestPayload_JSONBodyIsCachedAndStillBindable(t *testing.T) {
	responder := response.NewResponder(response.Options{Language: "en"}, i18n.NewTranslator())
	w, c := newPayloadContext(`{"name":"Jane"}`, "application/json")

	middleware.Payload(middleware.PayloadOptions{MaxBodySize: 1 << 20}, responder)(c)

	payload := middleware.GetPayload(c)
	if payload["name"] != "Jane" {
		t.Fatalf("GetPayload() = %v, want name=Jane", payload)
	}

	// The body must still be readable by a handler's own binding call.
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		t.Fatalf("ShouldBindJSON after Payload() error = %v, want the body to still be readable", err)
	}
	if body.Name != "Jane" {
		t.Fatalf("bound Name = %q, want %q", body.Name, "Jane")
	}
	_ = w
}

func TestPayload_FormBodyIsCached(t *testing.T) {
	responder := response.NewResponder(response.Options{Language: "en"}, i18n.NewTranslator())
	_, c := newPayloadContext("name=Jane&tag=a&tag=b", "application/x-www-form-urlencoded")

	middleware.Payload(middleware.PayloadOptions{MaxBodySize: 1 << 20}, responder)(c)

	payload := middleware.GetPayload(c)
	if payload["name"] != "Jane" {
		t.Fatalf(`GetPayload()["name"] = %v, want "Jane"`, payload["name"])
	}
	if tags, ok := payload["tag"].([]string); !ok || len(tags) != 2 {
		t.Fatalf(`GetPayload()["tag"] = %#v, want a 2-element []string (repeated form key)`, payload["tag"])
	}
}

func TestPayload_UnrecognizedContentTypeCachesRawBodyOnly(t *testing.T) {
	responder := response.NewResponder(response.Options{Language: "en"}, i18n.NewTranslator())
	_, c := newPayloadContext("plain text body", "text/plain")

	middleware.Payload(middleware.PayloadOptions{MaxBodySize: 1 << 20}, responder)(c)

	if payload := middleware.GetPayload(c); payload != nil {
		t.Fatalf("GetPayload() = %v, want nil for an unparsed content type", payload)
	}
}

func TestPayload_OversizedBodyIsRejected(t *testing.T) {
	responder := response.NewResponder(response.Options{Language: "en"}, i18n.NewTranslator())
	w, c := newPayloadContext(`{"name":"this body is way too long for the configured limit"}`, "application/json")

	middleware.Payload(middleware.PayloadOptions{MaxBodySize: 10}, responder)(c)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	if !c.IsAborted() {
		t.Error("an oversized body should abort the chain")
	}
}

func TestPayload_EmptyBodyPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	responder := response.NewResponder(response.Options{Language: "en"}, i18n.NewTranslator())
	middleware.Payload(middleware.PayloadOptions{MaxBodySize: 1 << 20}, responder)(c)

	if c.IsAborted() {
		t.Error("a request with no body should not be aborted")
	}
	if payload := middleware.GetPayload(c); payload != nil {
		t.Fatalf("GetPayload() = %v, want nil for an empty body", payload)
	}
}

func TestGetPayload_NothingCached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if payload := middleware.GetPayload(c); payload != nil {
		t.Fatalf("GetPayload() = %v, want nil when Payload() never ran", payload)
	}
}
