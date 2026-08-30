package cache

import (
	"context"
	"sync"
	"time"
)

// entry is one MemoryCache slot: a value plus its absolute expiry.
type entry[T any] struct {
	value    T
	expireAt time.Time
}

// expired reports whether the entry is past its expiry as of now.
func (e entry[T]) expired(now time.Time) bool {
	return !e.expireAt.IsZero() && now.After(e.expireAt)
}

// MemoryCache is an in-memory Cache implementation backed by a Go map.
type MemoryCache[T any] struct {
	mu    sync.RWMutex
	items map[string]entry[T]
}

// NewMemoryCache creates an empty MemoryCache.
//
// Usage:
//
//	c := cache.NewMemoryCache[*User]()
func NewMemoryCache[T any]() *MemoryCache[T] {
	return &MemoryCache[T]{items: make(map[string]entry[T])}
}

// Get implements Cache. An expired entry is treated as a miss and evicted.
func (c *MemoryCache[T]) Get(_ context.Context, key string) (T, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	var empty T
	if !ok {
		return empty, false
	}
	if item.expired(time.Now()) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return empty, false
	}
	return item.value, true
}

// Set implements Cache. A ttl of 0 stores the value with no expiration.
func (c *MemoryCache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	var expireAt time.Time
	if ttl > 0 {
		expireAt = time.Now().Add(ttl)
	}

	c.mu.Lock()
	c.items[key] = entry[T]{value: value, expireAt: expireAt}
	c.mu.Unlock()
	return nil
}

// Delete implements Cache.
func (c *MemoryCache[T]) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
	return nil
}

// Exists implements Cache.
func (c *MemoryCache[T]) Exists(ctx context.Context, key string) (bool, error) {
	_, found := c.Get(ctx, key)
	return found, nil
}

// GetOrSet implements Cache.
func (c *MemoryCache[T]) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFn func() (T, error)) (T, error) {
	if val, found := c.Get(ctx, key); found {
		return val, nil
	}

	val, err := fetchFn()
	if err != nil {
		var empty T
		return empty, err
	}

	_ = c.Set(ctx, key, val, ttl)
	return val, nil
}

// Close implements Cache. MemoryCache holds no external resources, so this is a no-op.
func (c *MemoryCache[T]) Close() error {
	return nil
}
