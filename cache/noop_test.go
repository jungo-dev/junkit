package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jungo-dev/junkit/cache"
)

func TestNoopCache(t *testing.T) {
	ctx := context.Background()
	c := cache.NewNoopCache[string]()

	if err := c.Set(ctx, "key", "value", time.Minute); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}

	if _, ok := c.Get(ctx, "key"); ok {
		t.Fatal("Get() reported a hit right after Set() — NoopCache must never retain anything")
	}

	if exists, err := c.Exists(ctx, "key"); err != nil || exists {
		t.Fatalf("Exists() = %v, %v, want false, nil", exists, err)
	}

	if err := c.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestNoopCache_GetOrSet(t *testing.T) {
	ctx := context.Background()
	c := cache.NewNoopCache[string]()

	t.Run("always calls fetchFn", func(t *testing.T) {
		called := false
		got, err := c.GetOrSet(ctx, "key", time.Minute, func() (string, error) {
			called = true
			return "fetched", nil
		})

		if err != nil {
			t.Fatalf("GetOrSet() error = %v", err)
		}
		if got != "fetched" {
			t.Fatalf("GetOrSet() = %q, want %q", got, "fetched")
		}
		if !called {
			t.Fatal("GetOrSet() did not call fetchFn")
		}

		if _, ok := c.Get(ctx, "key"); ok {
			t.Fatal("GetOrSet()'s result should not be retained")
		}
	})

	t.Run("propagates fetchFn's error", func(t *testing.T) {
		fetchErr := errors.New("upstream unavailable")

		_, err := c.GetOrSet(ctx, "key", time.Minute, func() (string, error) {
			return "", fetchErr
		})
		if !errors.Is(err, fetchErr) {
			t.Fatalf("GetOrSet() error = %v, want %v", err, fetchErr)
		}
	})
}
