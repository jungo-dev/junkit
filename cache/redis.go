package cache

import (
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisOptions configures a RedisCache's connection pool and timeouts.
type RedisOptions struct {
	// Addr is the Redis server address, e.g. "localhost:6379".
	Addr string
	// Password authenticates the connection; leave empty when auth is disabled.
	Password string
	// DB selects the Redis logical database (0-15).
	DB int

	// PoolSize is the maximum number of connections in the pool. Defaults to 10 * GOMAXPROCS.
	PoolSize int
	// MinIdleConns is the minimum number of idle connections kept open. Defaults to 5.
	MinIdleConns int

	// DialTimeout bounds connection establishment. Defaults to 5s.
	DialTimeout time.Duration
	// ReadTimeout bounds a single read. Defaults to 3s.
	ReadTimeout time.Duration
	// WriteTimeout bounds a single write. Defaults to 3s.
	WriteTimeout time.Duration

	// Logger receives non-fatal errors (marshal/connection failures). Optional.
	Logger Logger
}

// incrScript increments KEYS[1] and sets its expiry (ARGV[1] ms) if it has none, atomically.
var incrScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if tonumber(ARGV[1]) > 0 and redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// RedisCache is a generic Cache backed by Redis; values are JSON-encoded.
type RedisCache[T any] struct {
	client *redis.Client
	logger Logger
}

// NewRedisCache creates a RedisCache with opts.
//
// Usage:
//
//	c := cache.NewRedisCache[*User](cache.RedisOptions{Addr: "localhost:6379"})
//	defer c.Close()
func NewRedisCache[T any](opts RedisOptions) *RedisCache[T] {
	poolSize := opts.PoolSize
	if poolSize <= 0 {
		poolSize = 10 * runtime.GOMAXPROCS(0)
	}
	minIdleConns := opts.MinIdleConns
	if minIdleConns <= 0 {
		minIdleConns = 5
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	readTimeout := opts.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 3 * time.Second
	}
	writeTimeout := opts.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 3 * time.Second
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         opts.Addr,
		Password:     opts.Password,
		DB:           opts.DB,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	})

	return &RedisCache[T]{client: rdb, logger: opts.Logger}
}

// logError logs msg via the configured Logger, if any.
func (c *RedisCache[T]) logError(msg string, keysAndValues ...any) {
	if c.logger != nil {
		c.logger.Error(msg, keysAndValues...)
	}
}

// Ping checks whether the Redis server is reachable.
func (c *RedisCache[T]) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Get implements Cache.
func (c *RedisCache[T]) Get(ctx context.Context, key string) (T, bool) {
	var empty T

	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return empty, false
	} else if err != nil {
		c.logError("redis get error", "key", key, "error", err)
		return empty, false
	}

	var result T
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		c.logError("redis unmarshal error", "key", key, "error", err)
		return empty, false
	}
	return result, true
}

// Set implements Cache.
func (c *RedisCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) error {
	bytes, err := json.Marshal(value)
	if err != nil {
		c.logError("redis marshal error", "key", key, "error", err)
		return err
	}
	if err := c.client.Set(ctx, key, bytes, ttl).Err(); err != nil {
		c.logError("redis set error", "key", key, "error", err)
		return err
	}
	return nil
}

// Incr implements Cache.
func (c *RedisCache[T]) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return incrScript.Run(ctx, c.client, []string{key}, ttl.Milliseconds()).Int64()
}

// Delete implements Cache.
func (c *RedisCache[T]) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.logError("redis delete error", "key", key, "error", err)
		return err
	}
	return nil
}

// Exists implements Cache.
func (c *RedisCache[T]) Exists(ctx context.Context, key string) (bool, error) {
	val, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return val > 0, nil
}

// GetOrSet implements Cache.
func (c *RedisCache[T]) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFn func() (T, error)) (T, error) {
	if val, found := c.Get(ctx, key); found {
		return val, nil
	}

	val, err := fetchFn()
	if err != nil {
		var empty T
		return empty, err
	}

	if setErr := c.Set(ctx, key, val, ttl); setErr != nil {
		c.logError("redis set error in GetOrSet", "key", key, "error", setErr)
	}
	return val, nil
}

// Close implements Cache, closing the underlying connection pool.
func (c *RedisCache[T]) Close() error {
	return c.client.Close()
}

// IncrBy atomically increments the integer stored at key by value. The
// stored value must be an integer's string representation.
func (c *RedisCache[T]) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.IncrBy(ctx, key, value).Result()
}

// Expire sets a TTL on an existing key.
func (c *RedisCache[T]) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// SetNX atomically sets value at key only if the key does not already exist.
//
// Usage:
//
//	locked, _ := c.SetNX(ctx, "lock:order:456", true, 30*time.Second)
//	if locked {
//	    defer c.Delete(ctx, "lock:order:456")
//	}
func (c *RedisCache[T]) SetNX(ctx context.Context, key string, value T, ttl time.Duration) (bool, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return c.client.SetNX(ctx, key, bytes, ttl).Result()
}

// MGet retrieves multiple keys in a single round-trip.
func (c *RedisCache[T]) MGet(ctx context.Context, keys ...string) ([]T, []bool) {
	if len(keys) == 0 {
		return nil, nil
	}

	results := make([]T, len(keys))
	found := make([]bool, len(keys))

	vals, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		c.logError("redis mget error", "keys", keys, "error", err)
		return results, found
	}

	for i, val := range vals {
		str, ok := val.(string)
		if !ok {
			continue
		}
		var result T
		if err := json.Unmarshal([]byte(str), &result); err != nil {
			c.logError("redis mget unmarshal error", "key", keys[i], "error", err)
			continue
		}
		results[i] = result
		found[i] = true
	}
	return results, found
}

// MSet stores multiple key-value pairs in a single round-trip, all with the same ttl.
func (c *RedisCache[T]) MSet(ctx context.Context, items map[string]T, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()
	for key, value := range items {
		bytes, err := json.Marshal(value)
		if err != nil {
			return err
		}
		pipe.Set(ctx, key, bytes, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// DeleteByPattern scans and deletes all keys matching the glob pattern in batches.
func (c *RedisCache[T]) DeleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64

	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			pipe := c.client.Pipeline()
			for _, key := range keys {
				pipe.Del(ctx, key)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Clear removes all keys from the current Redis logical database.
func (c *RedisCache[T]) Clear(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// GetClient returns the underlying *redis.Client.
func (c *RedisCache[T]) GetClient() *redis.Client {
	return c.client
}
