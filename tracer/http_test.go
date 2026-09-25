package tracer_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jungo-dev/junkit/tracer"
)

func TestServerInfo_MasksSecretsAndReportsRequestDetails(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/users/1?foo=bar", nil)
	r.Host = "api.example.com:8080"
	r.Header.Set("Authorization", "Bearer abcdefghijklmnopqrstuvwxyz")
	r.Header.Set("Cookie", "session=super-secret; theme=dark")
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	r.Header.Set("User-Agent", "test-agent")

	info := tracer.ServerInfo(r)

	if info["method"] != http.MethodGet {
		t.Errorf(`info["method"] = %v, want %q`, info["method"], http.MethodGet)
	}
	if info["path"] != "/users/1" {
		t.Errorf(`info["path"] = %v, want %q`, info["path"], "/users/1")
	}
	if info["query_string"] != "foo=bar" {
		t.Errorf(`info["query_string"] = %v, want %q`, info["query_string"], "foo=bar")
	}
	if info["host"] != "api.example.com" || info["host_port"] != "8080" {
		t.Errorf(`info["host"], info["host_port"] = %v, %v, want "api.example.com", "8080"`, info["host"], info["host_port"])
	}
	if info["client_ip"] != "203.0.113.5" {
		t.Errorf(`info["client_ip"] = %v, want %q (first entry of X-Forwarded-For)`, info["client_ip"], "203.0.113.5")
	}
	if info["user_agent"] != "test-agent" {
		t.Errorf(`info["user_agent"] = %v, want %q`, info["user_agent"], "test-agent")
	}

	auth, _ := info["authorization"].(string)
	if auth == "Bearer abcdefghijklmnopqrstuvwxyz" {
		t.Error("authorization header should be masked, not passed through verbatim")
	}

	cookie, _ := info["cookie"].(string)
	if cookie == "session=super-secret; theme=dark" {
		t.Error("cookie header should be masked, not passed through verbatim")
	}
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantHost string
		wantPort string
	}{
		{name: "host with port", addr: "example.com:8080", wantHost: "example.com", wantPort: "8080"},
		{name: "bare host, no port", addr: "example.com", wantHost: "example.com", wantPort: ""},
		{name: "IPv4 with port", addr: "127.0.0.1:9000", wantHost: "127.0.0.1", wantPort: "9000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = tt.addr
			info := tracer.ServerInfo(r)

			if info["host"] != tt.wantHost {
				t.Errorf(`info["host"] = %v, want %q`, info["host"], tt.wantHost)
			}
			if info["host_port"] != tt.wantPort {
				t.Errorf(`info["host_port"] = %v, want %q`, info["host_port"], tt.wantPort)
			}
		})
	}
}

func TestHTTPInfo(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	tracer.HTTPInfo(r.Context(), r) // no Debugger: must not panic

	d := tracer.New()
	tracer.HTTPInfo(tracer.WithContext(r.Context(), d), r)

	logs := d.Logs()
	if len(logs) != 1 || logs[0].Type != tracer.TypeVariable || logs[0].Label != "REQUEST & SERVER INFO" {
		t.Fatalf("Logs() = %+v, want one REQUEST & SERVER INFO variable entry", logs)
	}
}
