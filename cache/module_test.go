package cache_test

import (
	"testing"

	"github.com/jungo-dev/junkit/cache"
)

func TestNew(t *testing.T) {
	t.Run("DriverMemory returns a MemoryCache", func(t *testing.T) {
		c := cache.New[string](cache.Options{Driver: cache.DriverMemory})
		if _, ok := c.(*cache.MemoryCache[string]); !ok {
			t.Fatalf("New() returned %T, want *cache.MemoryCache[string]", c)
		}
	})

	t.Run("DriverNoop returns a NoopCache", func(t *testing.T) {
		c := cache.New[string](cache.Options{Driver: cache.DriverNoop})
		if _, ok := c.(*cache.NoopCache[string]); !ok {
			t.Fatalf("New() returned %T, want *cache.NoopCache[string]", c)
		}
	})

	t.Run("a zero-value Options defaults to NoopCache", func(t *testing.T) {
		c := cache.New[string](cache.Options{})
		if _, ok := c.(*cache.NoopCache[string]); !ok {
			t.Fatalf("New() returned %T, want *cache.NoopCache[string]", c)
		}
	})

	t.Run("an unrecognized driver also defaults to NoopCache", func(t *testing.T) {
		c := cache.New[string](cache.Options{Driver: cache.Driver("bogus")})
		if _, ok := c.(*cache.NoopCache[string]); !ok {
			t.Fatalf("New() returned %T, want *cache.NoopCache[string]", c)
		}
	})

	t.Run("DriverRedis returns a RedisCache without connecting", func(t *testing.T) {
		c := cache.New[string](cache.Options{
			Driver: cache.DriverRedis,
			Redis:  cache.RedisOptions{Addr: "localhost:6379"},
		})
		defer c.Close()

		if _, ok := c.(*cache.RedisCache[string]); !ok {
			t.Fatalf("New() returned %T, want *cache.RedisCache[string]", c)
		}
	})
}
