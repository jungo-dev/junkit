// Package middleware provides Gin HTTP middleware: CORS, rate limiting, panic recovery, security headers, tracing, payload logging and authentication.
package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSOptions configures CORS middleware.
type CORSOptions struct {
	// AllowOrigins lists permitted origins; use "*" to allow any origin.
	AllowOrigins []string
	// AllowMethods lists permitted HTTP methods (e.g. GET, POST).
	AllowMethods []string
	// AllowHeaders lists permitted request headers.
	AllowHeaders []string
	// AllowCredentials sets Access-Control-Allow-Credentials header.
	AllowCredentials bool
	// MaxAge is the preflight cache duration in seconds (Access-Control-Max-Age).
	MaxAge int
}

// CORS creates a middleware that validates cross-origin requests.
//
// Usage:
//
//	router.Use(middleware.CORS(middleware.CORSOptions{
//	    AllowOrigins: []string{"https://app.example.com"},
//	    AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
//	    AllowHeaders: []string{"Authorization", "Content-Type"},
//	}))
func CORS(opts CORSOptions) gin.HandlerFunc {
	allowHeaders := strings.Join(opts.AllowHeaders, ", ")
	allowMethods := strings.Join(opts.AllowMethods, ", ")
	maxAge := strconv.Itoa(opts.MaxAge)
	allowAll := slices.Contains(opts.AllowOrigins, "*")

	return func(ctx *gin.Context) {
		origin := ctx.GetHeader("Origin")

		if origin != "" {
			isAllowed := allowAll || slices.Contains(opts.AllowOrigins, origin)
			if isAllowed {
				if allowAll && !opts.AllowCredentials {
					ctx.Writer.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					ctx.Writer.Header().Set("Access-Control-Allow-Origin", origin)
					ctx.Writer.Header().Set("Vary", "Origin")
				}
				if opts.AllowCredentials {
					ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
		}

		ctx.Writer.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", allowMethods)
		ctx.Writer.Header().Set("Access-Control-Max-Age", maxAge)

		if ctx.Request.Method == http.MethodOptions {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		ctx.Next()
	}
}
