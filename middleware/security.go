package middleware

import "github.com/gin-gonic/gin"

// Security creates a Gin middleware that attaches standard HTTP security headers to responses.
//
// Usage:
//
//	router.Use(middleware.Security())
func Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		c.Header("Cross-Origin-Resource-Policy", "cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Writer.Header().Del("Server")

		c.Next()
	}
}
