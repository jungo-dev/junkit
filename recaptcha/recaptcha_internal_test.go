package recaptcha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &Client{
		secretKey:  "test-secret",
		httpClient: server.Client(),
		minScore:   0.5,
		apiURL:     server.URL,
	}
}

func TestClient_Verify_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success": true, "score": 0.9}`))
	})

	if err := c.Verify(context.Background(), "token", "1.2.3.4"); err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
}

func TestClient_Verify_FailedVerification(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success": false, "error-codes": ["invalid-input-response"]}`))
	})

	err := c.Verify(context.Background(), "bad-token", "")
	if err == nil {
		t.Fatal("Verify() error = nil, want an error when success is false")
	}
	if !strings.Contains(err.Error(), "invalid-input-response") {
		t.Fatalf("error = %v, want it to mention the error code", err)
	}
}

func TestClient_Verify_ScoreTooLow(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success": true, "score": 0.1}`))
	})

	err := c.Verify(context.Background(), "token", "")
	if err == nil {
		t.Fatal("Verify() error = nil, want an error when the score is below the minimum")
	}
	if !strings.Contains(err.Error(), "score too low") {
		t.Fatalf("error = %v, want it to mention the low score", err)
	}
}

func TestClient_Verify_NonOKStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if err := c.Verify(context.Background(), "token", ""); err == nil {
		t.Fatal("Verify() error = nil, want an error for a non-200 API response")
	}
}

func TestClient_Verify_SendsRemoteIPWhenProvided(t *testing.T) {
	var gotRemoteIP string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotRemoteIP = r.URL.Query().Get("remoteip")
		w.Write([]byte(`{"success": true, "score": 0.9}`))
	})

	if err := c.Verify(context.Background(), "token", "203.0.113.5"); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if gotRemoteIP != "203.0.113.5" {
		t.Fatalf("server received remoteip=%q, want %q", gotRemoteIP, "203.0.113.5")
	}
}

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient(Options{SecretKey: "x"}, nil)

	if c.minScore != 0.5 {
		t.Errorf("minScore = %v, want the documented default of 0.5", c.minScore)
	}
	if c.httpClient != http.DefaultClient {
		t.Error("httpClient should default to http.DefaultClient when nil is passed")
	}
	if c.apiURL != "https://www.google.com/recaptcha/api/siteverify" {
		t.Errorf("apiURL = %q, want Google's real siteverify endpoint", c.apiURL)
	}
}

func TestNewClient_NegativeMinScoreAlsoDefaults(t *testing.T) {
	c := NewClient(Options{SecretKey: "x", MinScore: -1}, nil)
	if c.minScore != 0.5 {
		t.Errorf("minScore = %v, want the default of 0.5 for a non-positive MinScore", c.minScore)
	}
}

func TestClient_Verify_MissingSecretKey(t *testing.T) {
	c := &Client{secretKey: "", httpClient: http.DefaultClient, minScore: 0.5, apiURL: "http://unused"}

	if err := c.Verify(context.Background(), "token", ""); err == nil {
		t.Fatal("Verify() error = nil, want an error when no secret key is configured")
	}
}
