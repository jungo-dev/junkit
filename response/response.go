package response

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/pagination"
)

// Context keys set by middleware.Trace and read to construct APIInfo.
const (
	StartTimeContextKey = "start_time"
	TraceIDContextKey   = "trace_id"
	VersionContextKey   = "api_version"
	EnvContextKey       = "api_env"
)

// PaginatedData wraps a page of items with Pagination metadata.
type PaginatedData struct {
	Items      any                   `json:"items"`
	Pagination pagination.Pagination `json:"pagination,omitempty"`
}

// Response defines the standard JSON API response envelope.
type Response struct {
	Status     string    `json:"status"`
	Code       ErrorCode `json:"code,omitempty"`
	Message    string    `json:"message,omitempty"`
	Data       any       `json:"data,omitempty"`
	Error      any       `json:"error,omitempty"`
	Pagination any       `json:"pagination,omitempty"`
	APIInfo    *APIInfo  `json:"api_info,omitempty"`
}

// APIInfo carries request diagnostic metadata (trace ID, timestamp, latency, version).
type APIInfo struct {
	TraceId   string `json:"trace_id,omitempty" example:"eb9b3d81-869d-4b62-80b8-5780f2da2f30"`
	Timestamp string `json:"timestamp" example:"2026-01-10T12:44:05Z"`
	Latency   string `json:"latency,omitempty" example:"188.280ms"`
	Version   string `json:"version,omitempty" example:"v1.0.0"`
	Env       string `json:"env,omitempty" example:"production"`
}

// getAPIInfo constructs APIInfo from values stored in the Gin context.
func getAPIInfo(ctx *gin.Context) *APIInfo {
	info := &APIInfo{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if startTime, exists := ctx.Get(StartTimeContextKey); exists {
		if t, ok := startTime.(time.Time); ok {
			info.Latency = fmt.Sprintf("%.3fms", float64(time.Since(t).Microseconds())/1000.0)
		}
	}
	if val, ok := ctx.Get(TraceIDContextKey); ok {
		info.TraceId, _ = val.(string)
	}
	if val, ok := ctx.Get(VersionContextKey); ok {
		info.Version, _ = val.(string)
	}
	if val, ok := ctx.Get(EnvContextKey); ok {
		info.Env, _ = val.(string)
	}

	return info
}
