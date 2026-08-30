package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/response"
)

// Gin context keys Payload populates.
const (
	// PayloadKey holds the request body parsed as map[string]any (JSON or form).
	PayloadKey = "_request_payload"
	// PayloadRawKey holds the raw request body bytes.
	PayloadRawKey = "_request_body_raw"
)

// PayloadOptions configures request body reading rules.
type PayloadOptions struct {
	// MaxBodySize is the maximum allowed request body size in bytes.
	MaxBodySize int64
}

// Payload reads, limits, and caches the request body in Gin context for downstream handlers.
//
// Usage:
//
//	router.Use(middleware.Payload(middleware.PayloadOptions{MaxBodySize: 2 << 20}, responder))
func Payload(opts PayloadOptions, responder response.Responder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.ContentLength == 0 {
			c.Next()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, opts.MaxBodySize)

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			responder.Send(c, http.StatusRequestEntityTooLarge, "request_body_too_large")
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		c.Set(PayloadRawKey, bodyBytes)

		if payload := parsePayload(c, bodyBytes); payload != nil {
			c.Set(PayloadKey, payload)
		}

		c.Next()
	}
}

// parsePayload parses JSON or form-urlencoded request body into a map.
func parsePayload(c *gin.Context, bodyBytes []byte) map[string]any {
	contentType := c.GetHeader("Content-Type")

	if strings.Contains(contentType, "application/json") {
		var payload map[string]any
		if json.Unmarshal(bodyBytes, &payload) == nil {
			return payload
		}
		return nil
	}

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := c.Request.ParseForm(); err != nil {
			return nil
		}
		payload := make(map[string]any, len(c.Request.PostForm))
		for k, v := range c.Request.PostForm {
			if len(v) == 1 {
				payload[k] = v[0]
			} else {
				payload[k] = v
			}
		}
		return payload
	}

	return nil
}

// GetPayload returns the payload cached by Payload, or nil if none was cached.
func GetPayload(c *gin.Context) map[string]any {
	if val, ok := c.Get(PayloadKey); ok {
		if payload, ok := val.(map[string]any); ok {
			return payload
		}
	}
	return nil
}
