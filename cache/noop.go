package cache

import (
	"context"
	"time"
)

// NoopCache is a no-op Cache implementation that disables caching: every read misses and writes are ignored.
type NoopCache[T any] struct{}

// NewNoopCache creates a new NoopCache instance.
func NewNoopCache[T any]() *NoopCache[T] {
	return &NoopCache[T]{}
}

// Get always reports a miss.
func (c *NoopCache[T]) Get(context.Context, string) (T, bool) {
	var empty T
	return empty, false
}

// Set is a no-op that always succeeds.
func (c *NoopCache[T]) Set(context.Context, string, T, time.Duration) error {
	return nil
}

// Incr always returns 0.
func (c *NoopCache[T]) Incr(context.Context, string, time.Duration) (int64, error) {
	return 0, nil
}

// Delete is a no-op that always succeeds.
func (c *NoopCache[T]) Delete(context.Context, string) error {
	return nil
}

// Exists always reports false.
func (c *NoopCache[T]) Exists(context.Context, string) (bool, error) {
	return false, nil
}

// GetOrSet always calls fetchFn — every read is a miss — and discards its result.
func (c *NoopCache[T]) GetOrSet(_ context.Context, _ string, _ time.Duration, fetchFn func() (T, error)) (T, error) {
	return fetchFn()
}

// Close is a no-op.
func (c *NoopCache[T]) Close() error {
	return nil
}
