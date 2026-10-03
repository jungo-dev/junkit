package security

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
)

// identityKey is the private context key for the authenticated identity.
type identityKey struct{}

// SetIdentity stores identity in the gin.Context and its request context.
//
// Usage:
//
//	security.SetIdentity(c, identity)
func SetIdentity[T any](c *gin.Context, identity T) {
	c.Set(identityKey{}, identity)
	if c.Request != nil {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), identityKey{}, identity))
	}
}

// WithIdentity returns a copy of ctx carrying identity (for tests and background work).
//
// Usage:
//
//	ctx := security.WithIdentity(context.Background(), identity)
func WithIdentity[T any](ctx context.Context, identity T) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// GetIdentity returns the identity of type T from a *gin.Context or context.Context.
//
// Usage:
//
//	identity, ok := security.GetIdentity[*domain.Identity](ctx)
func GetIdentity[T any](ctx context.Context) (T, bool) {
	if gc, ok := ctx.(*gin.Context); ok {
		if val, exists := gc.Get(identityKey{}); exists {
			if identity, ok := val.(T); ok {
				return identity, true
			}
		}
		if gc.Request != nil {
			ctx = gc.Request.Context()
		}
	}

	identity, ok := ctx.Value(identityKey{}).(T)
	return identity, ok
}

// MustGetIdentity is GetIdentity for routes behind an auth middleware; it panics when missing.
//
// Usage:
//
//	identity := security.MustGetIdentity[*domain.Identity](c)
func MustGetIdentity[T any](ctx context.Context) T {
	identity, ok := GetIdentity[T](ctx)
	if !ok {
		var zero T
		panic(fmt.Sprintf("security: no identity of type %T in context", zero))
	}
	return identity
}
