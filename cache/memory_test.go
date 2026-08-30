package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jungo-dev/junkit/cache"
)

func TestMemoryCache_GetSet(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache[string]()

	if _, ok := c.Get(ctx, "missing"); ok {
		t.Fatal("Get() on an empty cache reported a hit")
	}

	if err := c.Set(ctx, "key", "value", 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, ok := c.Get(ctx, "key")
	if !ok {
		t.Fatal("Get() after Set() reported a miss")
	}
	if got != "value" {
		t.Fatalf("Get() = %q, want %q", got, "value")
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache[int]()

	_ = c.Set(ctx, "key", 42, 0)
	if err := c.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, ok := c.Get(ctx, "key"); ok {
		t.Fatal("Get() after Delete() reported a hit")
	}

	// Deleting an already-absent key is not an error.
	if err := c.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() on a missing key error = %v, want nil", err)
	}
}

func TestMemoryCache_Exists(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache[string]()

	if exists, err := c.Exists(ctx, "key"); err != nil || exists {
		t.Fatalf("Exists() = %v, %v, want false, nil", exists, err)
	}

	_ = c.Set(ctx, "key", "value", 0)

	if exists, err := c.Exists(ctx, "key"); err != nil || !exists {
		t.Fatalf("Exists() = %v, %v, want true, nil", exists, err)
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache[string]()

	if err := c.Set(ctx, "key", "value", 20*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, ok := c.Get(ctx, "key"); !ok {
		t.Fatal("Get() immediately after Set() with a TTL reported a miss")
	}

	time.Sleep(60 * time.Millisecond)

	if _, ok := c.Get(ctx, "key"); ok {
		t.Fatal("Get() after the TTL elapsed reported a hit")
	}
}

func TestMemoryCache_GetOrSet(t *testing.T) {
	ctx := context.Background()

	t.Run("a hit never calls fetchFn", func(t *testing.T) {
		c := cache.NewMemoryCache[string]()
		_ = c.Set(ctx, "key", "cached", 0)

		called := false
		got, err := c.GetOrSet(ctx, "key", 0, func() (string, error) {
			called = true
			return "fetched", nil
		})

		if err != nil {
			t.Fatalf("GetOrSet() error = %v", err)
		}
		if got != "cached" {
			t.Fatalf("GetOrSet() = %q, want the cached value %q", got, "cached")
		}
		if called {
			t.Fatal("GetOrSet() called fetchFn on a cache hit")
		}
	})

	t.Run("a miss calls fetchFn and caches its result", func(t *testing.T) {
		c := cache.NewMemoryCache[string]()

		got, err := c.GetOrSet(ctx, "key", 0, func() (string, error) {
			return "fetched", nil
		})
		if err != nil {
			t.Fatalf("GetOrSet() error = %v", err)
		}
		if got != "fetched" {
			t.Fatalf("GetOrSet() = %q, want %q", got, "fetched")
		}

		cached, ok := c.Get(ctx, "key")
		if !ok || cached != "fetched" {
			t.Fatalf("value was not cached after GetOrSet: Get() = %q, %v", cached, ok)
		}
	})

	t.Run("fetchFn's error is returned and nothing is cached", func(t *testing.T) {
		c := cache.NewMemoryCache[string]()
		fetchErr := errors.New("upstream unavailable")

		_, err := c.GetOrSet(ctx, "key", 0, func() (string, error) {
			return "", fetchErr
		})
		if !errors.Is(err, fetchErr) {
			t.Fatalf("GetOrSet() error = %v, want %v", err, fetchErr)
		}

		if _, ok := c.Get(ctx, "key"); ok {
			t.Fatal("a failed fetchFn should not populate the cache")
		}
	})
}

func TestMemoryCache_Close(t *testing.T) {
	c := cache.NewMemoryCache[string]()
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}
