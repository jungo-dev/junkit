// Package cache provides a generic key-value cache supporting Redis, in-memory,
// and no-op backends.
package cache

import (
	"context"
	"time"
)

// Logger defines a simple interface for logging non-fatal cache errors.
type Logger interface {
	Error(msg string, keysAndValues ...any)
}

// Cache is the generic key-value cache interface implemented by RedisCache,
// MemoryCache, and NoopCache.
type Cache[T any] interface {
	// Get retrieves a value by key. Returns (value, true) if found, or (zero-value, false) if missing.
	Get(ctx context.Context, key string) (T, bool)
	// Set saves a key-value pair with expiration duration ttl (ttl=0 means no expiration).
	Set(ctx context.Context, key string, value T, ttl time.Duration) error
	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error
	// Exists checks if a key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)
	// GetOrSet returns the cached value, or calls fetchFn on cache miss to fetch and store it.
	GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFn func() (T, error)) (T, error)
	// Close releases any underlying resources (e.g. connections).
	Close() error
}
