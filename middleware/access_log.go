package middleware

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/jungo-dev/junkit/response"
)

// defaultMaxAccessLogBodySize caps the response body size buffered for logging.
const defaultMaxAccessLogBodySize = 100 * 1024

// defaultExcludedPaths are paths skipped by AccessLog by default.
var defaultExcludedPaths = []string{"/health", "/metrics", "/favicon.ico"}

// defaultSensitiveFields are payload fields masked by default.
var defaultSensitiveFields = []string{
	"password", "pass", "token", "new_password",
	"access_token", "refresh_token", "secret", "authorization",
	"recaptcha_token",
}

var accessLogHostname, _ = os.Hostname()

// accessLogBufferPool reuses response body buffers to reduce allocations.
var accessLogBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// AccessLogOptions configures AccessLog middleware.
type AccessLogOptions struct {
	// ExcludedPaths lists request paths to skip. Defaults to defaultExcludedPaths.
	ExcludedPaths []string
	// SensitiveFields lists JSON field names to mask (case-insensitive). Defaults to defaultSensitiveFields.
	SensitiveFields []string
	// MaxBodySize caps response bytes buffered for logging. Defaults to 100KB.
	MaxBodySize int
	// SkipIf dynamically skips logging for a request when returning true.
	SkipIf func(*gin.Context) bool
}

// accessLogResponseWriter wraps gin.ResponseWriter to buffer the response body.
type accessLogResponseWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	maxSize int
}

func (w *accessLogResponseWriter) Write(data []byte) (int, error) {
	if w.body.Len() < w.maxSize {
		remaining := w.maxSize - w.body.Len()
		if len(data) > remaining {
			w.body.Write(data[:remaining])
		} else {
			w.body.Write(data)
		}
	}
	return w.ResponseWriter.Write(data)
}

// AccessLog returns a middleware that logs structured HTTP request/response details.
//
// Usage:
//
//	router.Use(middleware.AccessLog(httpLogger, middleware.AccessLogOptions{}))
func AccessLog(httpLogger *zap.Logger, opts AccessLogOptions) gin.HandlerFunc {
	excluded := opts.ExcludedPaths
	if len(excluded) == 0 {
		excluded = defaultExcludedPaths
	}
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, p := range excluded {
		excludedSet[p] = struct{}{}
	}

	sensitive := opts.SensitiveFields
	if len(sensitive) == 0 {
		sensitive = defaultSensitiveFields
	}
	sensitiveSet := make(map[string]struct{}, len(sensitive))
	for _, f := range sensitive {
		sensitiveSet[strings.ToLower(f)] = struct{}{}
	}

	maxBodySize := opts.MaxBodySize
	if maxBodySize <= 0 {
		maxBodySize = defaultMaxAccessLogBodySize
	}

	return func(c *gin.Context) {
		if _, ok := excludedSet[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		if opts.SkipIf != nil && opts.SkipIf(c) {
			c.Next()
			return
		}

		start := time.Now()

		respBuf := accessLogBufferPool.Get().(*bytes.Buffer)
		respBuf.Reset()
		defer accessLogBufferPool.Put(respBuf)

		c.Writer = &accessLogResponseWriter{ResponseWriter: c.Writer, body: respBuf, maxSize: maxBodySize}

		c.Next()

		duration := time.Since(start)

		reqBody := GetPayload(c)
		sanitizeMapInPlace(reqBody, sensitiveSet)

		respBody := parseAccessLogResponseBody(c.Writer.Header().Get("Content-Type"), respBuf.Bytes())
		if m, ok := respBody.(map[string]any); ok {
			sanitizeMapInPlace(m, sensitiveSet)
		}

		var errs any
		if len(c.Errors) > 0 {
			errs = c.Errors.ByType(gin.ErrorTypePrivate).String()
		}

		traceID, _ := c.Get(response.TraceIDContextKey)

		logFn := accessLogFuncForStatus(httpLogger, c.Writer.Status())
		logFn("HTTP_ACCESS_LOG",
			zap.Any("trace_id", traceID),
			zap.String("hostname", accessLogHostname),
			zap.Int64("latency_ms", duration.Milliseconds()),
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("resp_size", c.Writer.Size()),
			zap.Any("error", errs),
			zap.Any("request_payload", reqBody),
			zap.Any("response_payload", respBody),
			zap.Time("request_time", start),
		)
	}
}

// accessLogFuncForStatus returns the log level function based on HTTP status.
func accessLogFuncForStatus(logger *zap.Logger, status int) func(string, ...zap.Field) {
	switch {
	case status >= 500:
		return logger.Error
	case status >= 400:
		return logger.Warn
	default:
		return logger.Info
	}
}

// parseAccessLogResponseBody parses JSON data or returns a truncated string.
func parseAccessLogResponseBody(contentType string, data []byte) any {
	if len(data) == 0 {
		return nil
	}

	if strings.Contains(contentType, "application/json") {
		var result any
		if err := json.Unmarshal(data, &result); err == nil {
			return result
		}
	}

	if len(data) > 200 {
		return string(data[:200]) + "... [truncated]"
	}
	return string(data)
}

// sanitizeMapInPlace recursively masks sensitive fields in a map.
func sanitizeMapInPlace(data map[string]any, sensitive map[string]struct{}) {
	for k, v := range data {
		if _, ok := sensitive[strings.ToLower(k)]; ok {
			data[k] = "[MASKED]"
			continue
		}

		switch val := v.(type) {
		case map[string]any:
			sanitizeMapInPlace(val, sensitive)
		case []any:
			for _, item := range val {
				if m, ok := item.(map[string]any); ok {
					sanitizeMapInPlace(m, sensitive)
				}
			}
		}
	}
}
