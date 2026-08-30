package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/jungo-dev/junkit/notification"
	"github.com/jungo-dev/junkit/response"
)

// LimiterOptions configures rate limiting rules for a client IP.
type LimiterOptions struct {
	// RequestsPerSecond is the sustained rate allowed per client IP.
	RequestsPerSecond int
	// Burst is the maximum burst size allowed above RequestsPerSecond.
	Burst int

	// CleanupInterval is how often idle client entries are swept.
	CleanupInterval time.Duration
	// ClientTTL is how long a client entry survives with no requests before eviction.
	ClientTTL time.Duration
	// LogTTL throttles repeated "rate limit exceeded" log/alert lines per IP to at most once per this interval.
	LogTTL time.Duration
	// NumShards controls lock striping across the client map (defaults to 64 when <= 0).
	NumShards int
	// IPWhitelist lists client IPs that always bypass the limiter.
	IPWhitelist []string
}

// client tracks one visitor's rate limiter and last-seen time.
type client struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64
}

// shard represents a lock-striped subset of rate limiters to reduce lock contention.
type shard struct {
	mu      sync.RWMutex
	clients map[string]*client
}

// ClientStore manages per-IP rate limiters across sharded maps.
type ClientStore struct {
	shards          []*shard
	numShards       int
	cleanupInterval time.Duration
	clientTTL       time.Duration
	rps             int
	burst           int
	once            sync.Once
}

// NewClientStore creates a new ClientStore instance configured by opts.
func NewClientStore(opts LimiterOptions) *ClientStore {
	numShards := opts.NumShards
	if numShards <= 0 {
		numShards = 64
	}

	s := &ClientStore{
		shards:          make([]*shard, numShards),
		numShards:       numShards,
		cleanupInterval: opts.CleanupInterval,
		clientTTL:       opts.ClientTTL,
		rps:             opts.RequestsPerSecond,
		burst:           opts.Burst,
	}
	for i := range s.shards {
		s.shards[i] = &shard{clients: make(map[string]*client)}
	}
	return s
}

// getShard selects ip's shard via a simple FNV-like hash.
func (s *ClientStore) getShard(ip string) *shard {
	var hash uint32 = 2166136261
	for i := 0; i < len(ip); i++ {
		hash ^= uint32(ip[i])
		hash *= 16777619
	}
	return s.shards[hash%uint32(s.numShards)]
}

// getLimiter returns or creates a rate.Limiter for the given IP address.
func (s *ClientStore) getLimiter(ip string) *rate.Limiter {
	sh := s.getShard(ip)
	now := time.Now()

	sh.mu.RLock()
	c, exists := sh.clients[ip]
	sh.mu.RUnlock()

	if exists {
		lastSeenTime := time.Unix(0, c.lastSeen.Load())
		if now.Sub(lastSeenTime) > 10*time.Second {
			c.lastSeen.Store(now.UnixNano())
		}
		return c.limiter
	}

	sh.mu.Lock()
	defer sh.mu.Unlock()

	if c, exists := sh.clients[ip]; exists {
		return c.limiter
	}

	limiter := rate.NewLimiter(rate.Limit(s.rps), s.burst)
	newClient := &client{limiter: limiter}
	newClient.lastSeen.Store(now.UnixNano())
	sh.clients[ip] = newClient
	return limiter
}

// StartCleanup starts the background goroutine that evicts idle client limiters.
func (s *ClientStore) StartCleanup() {
	s.once.Do(func() {
		go func() {
			ticker := time.NewTicker(s.cleanupInterval)
			defer ticker.Stop()

			for range ticker.C {
				for i := 0; i < s.numShards; i++ {
					s.cleanupShard(i)
				}
			}
		}()
	})
}

// cleanupShard evicts entries in shard i idle past ClientTTL.
func (s *ClientStore) cleanupShard(i int) {
	sh := s.shards[i]
	now := time.Now()

	sh.mu.Lock()
	defer sh.mu.Unlock()

	for ip, c := range sh.clients {
		if now.Sub(time.Unix(0, c.lastSeen.Load())) > s.clientTTL {
			delete(sh.clients, ip)
		}
	}
}

// rateLimitLogCache throttles log and alert notifications per IP address.
type rateLimitLogCache struct {
	cache  sync.Map
	logTTL time.Duration
}

// startCleanup periodically evicts cache entries older than 2x logTTL.
func (lc *rateLimitLogCache) startCleanup() {
	go func() {
		ticker := time.NewTicker(lc.logTTL * 2)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			lc.cache.Range(func(key, value any) bool {
				if t, ok := value.(time.Time); ok && now.Sub(t) > lc.logTTL {
					lc.cache.Delete(key)
				}
				return true
			})
		}
	}()
}

// shouldLog checks whether a log or alert can be emitted for the given IP.
func (lc *rateLimitLogCache) shouldLog(ip string) bool {
	now := time.Now()

	val, loaded := lc.cache.LoadOrStore(ip, now)
	if !loaded {
		return true
	}

	last, ok := val.(time.Time)
	if !ok || now.Sub(last) >= lc.logTTL {
		lc.cache.Store(ip, now)
		return true
	}
	return false
}

// Limiter creates a Gin middleware that rate-limits incoming HTTP requests per client IP.
//
// Usage:
//
//	router.Use(middleware.Limiter(middleware.LimiterOptions{
//	    RequestsPerSecond: 10, Burst: 20, CleanupInterval: time.Minute,
//	    ClientTTL: 10 * time.Minute, LogTTL: time.Minute,
//	}, zapLogger, responder, notifier))
func Limiter(opts LimiterOptions, logger *zap.Logger, responder response.Responder, notifier notification.Notifier) gin.HandlerFunc {
	store := NewClientStore(opts)
	store.StartCleanup()

	whitelistMap := make(map[string]struct{}, len(opts.IPWhitelist))
	for _, ip := range opts.IPWhitelist {
		whitelistMap[ip] = struct{}{}
	}

	logCache := &rateLimitLogCache{logTTL: opts.LogTTL}
	logCache.startCleanup()

	return func(ctx *gin.Context) {
		ip := ctx.ClientIP()

		if _, ok := whitelistMap[ip]; ok {
			ctx.Next()
			return
		}

		userAgent := ctx.Request.UserAgent()
		if strings.HasPrefix(userAgent, "PostmanRuntime/") || strings.HasPrefix(userAgent, "GoHttpTest/") {
			ctx.Next()
			return
		}

		if !store.getLimiter(ip).Allow() {
			if logCache.shouldLog(ip) {
				logger.Warn("rate limiter exceeded",
					zap.String("ip", ip),
					zap.String("path", ctx.Request.URL.Path),
					zap.Int("limit_rps", opts.RequestsPerSecond),
				)

				if notifier != nil {
					msg := rateLimitAlertMessage(ctx, ip)
					ctxCopy := ctx.Copy()
					go notifier.SendMessageHTML(ctxCopy, msg, "error")
				}
			}

			responder.Send(ctx, http.StatusTooManyRequests, "too_many_requests")
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// rateLimitAlertMessage formats an HTML alert message for rate limit breaches.
func rateLimitAlertMessage(ctx *gin.Context, ip string) string {
	return fmt.Sprintf(`
	🚦 <b>Rate Limit Exceeded</b>

	<b>Request Info</b>
	• <b>IP:</b> <code>%s</code>
	• <b>Method:</b> <code>%s</code>
	• <b>Path:</b> <code>%s</code>
	• <b>Time:</b> <code>%s</code>

	⚠️ <i>Please check and block this IP address if necessary.</i>
	`,
		ip,
		ctx.Request.Method,
		ctx.Request.URL.Path,
		time.Now().Format(time.RFC3339),
	)
}
