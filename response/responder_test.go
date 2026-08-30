package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/i18n"
	"github.com/jungo-dev/junkit/pagination"
	"github.com/jungo-dev/junkit/response"
)

func newResponderTestContext() (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return w, c
}

func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var res response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}
	return res
}

func TestResponder_Send(t *testing.T) {
	translator := i18n.NewTranslator()
	responder := response.NewResponder(response.Options{Language: "en"}, translator)
	w, c := newResponderTestContext()

	responder.Send(c, http.StatusOK, "operation_successful")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	res := decodeResponse(t, w)
	if res.Status != "success" {
		t.Fatalf("Status = %q, want %q", res.Status, "success")
	}
	if res.Message != "Operation successful" {
		t.Fatalf("Message = %q, want the resolved translation", res.Message)
	}
	if res.APIInfo == nil {
		t.Fatal("APIInfo should always be populated")
	}
}

func TestResponder_SendWithData_RoutesByStatusCode(t *testing.T) {
	translator := i18n.NewTranslator()
	responder := response.NewResponder(response.Options{Language: "en"}, translator)

	t.Run("success status uses the data field", func(t *testing.T) {
		w, c := newResponderTestContext()
		responder.SendWithData(c, http.StatusOK, "operation_successful", map[string]string{"id": "1"})

		res := decodeResponse(t, w)
		if res.Data == nil {
			t.Fatal("Data should be set for a 2xx status")
		}
		if res.Error != nil {
			t.Fatalf("Error = %v, want nil for a 2xx status", res.Error)
		}
	})

	t.Run("error status uses the error field", func(t *testing.T) {
		w, c := newResponderTestContext()
		responder.SendWithData(c, http.StatusUnprocessableEntity, "validation_error", map[string]string{"email": "required"})

		res := decodeResponse(t, w)
		if res.Error == nil {
			t.Fatal("Error should be set for a 4xx status")
		}
		if res.Data != nil {
			t.Fatalf("Data = %v, want nil for a 4xx status", res.Data)
		}
	})
}

func TestResponder_Pagination(t *testing.T) {
	translator := i18n.NewTranslator()
	responder := response.NewResponder(response.Options{Language: "en"}, translator)
	w, c := newResponderTestContext()

	meta := pagination.NewPagination(1, 10, 25)
	responder.Pagination(c, http.StatusOK, "operation_successful", []string{"a", "b"}, meta)

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	data, ok := raw["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want an object with items/pagination", raw["data"])
	}
	if _, ok := data["items"]; !ok {
		t.Fatal(`data["items"] missing`)
	}
	if _, ok := data["pagination"]; !ok {
		t.Fatal(`data["pagination"] missing`)
	}
}

func TestResponder_Translate(t *testing.T) {
	translator := i18n.NewTranslator()
	translator.AddTranslations(map[string]map[string]string{
		i18n.LangEN: {"greeting": "Hello, %s!"},
	})
	responder := response.NewResponder(response.Options{Language: "en"}, translator)

	if got := responder.Translate("greeting", "Jane"); got != "Hello, Jane!" {
		t.Fatalf("Translate() = %q, want %q", got, "Hello, Jane!")
	}
}

func TestResponder_Error_AppError(t *testing.T) {
	translator := i18n.NewTranslator()
	translator.AddTranslations(map[string]map[string]string{
		i18n.LangEN: {"user_not_found": "User not found"},
	})
	responder := response.NewResponder(response.Options{Language: "en"}, translator)
	w, c := newResponderTestContext()

	responder.Error(c, response.New(response.NotFound, "user_not_found"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	res := decodeResponse(t, w)
	if res.Code != response.NotFound {
		t.Fatalf("Code = %q, want %q", res.Code, response.NotFound)
	}
	if res.Message != "User not found" {
		t.Fatalf("Message = %q, want the resolved translation", res.Message)
	}
}

func TestResponder_Error_OrdinaryErrorBecomesGeneric500(t *testing.T) {
	translator := i18n.NewTranslator()
	responder := response.NewResponder(response.Options{Language: "en"}, translator)
	w, c := newResponderTestContext()

	responder.Error(c, errors500Sentinel)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d for a non-AppError", w.Code, http.StatusInternalServerError)
	}
	if len(c.Errors) != 1 {
		t.Fatalf("gin context Errors = %v, want the raw error recorded for logging middleware", c.Errors)
	}
}

var errors500Sentinel = &fakeError{"a plain, unstructured error"}

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }

func TestResponder_Error_MapsEveryErrorCode(t *testing.T) {
	tests := []struct {
		code       response.ErrorCode
		wantStatus int
	}{
		{response.BadRequest, http.StatusBadRequest},
		{response.Unauthorized, http.StatusUnauthorized},
		{response.NotFound, http.StatusNotFound},
		{response.Forbidden, http.StatusForbidden},
		{response.Conflict, http.StatusConflict},
		{response.UnsupportedMediaType, http.StatusUnsupportedMediaType},
		{response.TooManyRequests, http.StatusTooManyRequests},
		{response.UnprocessableEntity, http.StatusUnprocessableEntity},
		{response.Internal, http.StatusInternalServerError},
		{response.ErrorCode("SOMETHING_UNRECOGNIZED"), http.StatusInternalServerError},
	}

	translator := i18n.NewTranslator()
	responder := response.NewResponder(response.Options{Language: "en"}, translator)

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			w, c := newResponderTestContext()
			responder.Error(c, response.New(tt.code, "some_key"))

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d for code %q", w.Code, tt.wantStatus, tt.code)
			}
		})
	}
}

func TestGetAPIInfo_ReadsTraceMiddlewareContext(t *testing.T) {
	translator := i18n.NewTranslator()
	responder := response.NewResponder(response.Options{Language: "en"}, translator)
	w, c := newResponderTestContext()

	c.Set(response.TraceIDContextKey, "trace-123")
	c.Set(response.VersionContextKey, "v1.2.3")
	c.Set(response.EnvContextKey, "production")
	c.Set(response.StartTimeContextKey, time.Now().Add(-50*time.Millisecond))

	responder.Send(c, http.StatusOK, "operation_successful")

	res := decodeResponse(t, w)
	if res.APIInfo.TraceId != "trace-123" {
		t.Errorf("APIInfo.TraceId = %q, want %q", res.APIInfo.TraceId, "trace-123")
	}
	if res.APIInfo.Version != "v1.2.3" {
		t.Errorf("APIInfo.Version = %q, want %q", res.APIInfo.Version, "v1.2.3")
	}
	if res.APIInfo.Env != "production" {
		t.Errorf("APIInfo.Env = %q, want %q", res.APIInfo.Env, "production")
	}
	if res.APIInfo.Latency == "" {
		t.Error("APIInfo.Latency should be computed when start_time is present")
	}
}
