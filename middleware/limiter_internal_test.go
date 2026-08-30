package middleware

import (
	"testing"
	"time"
)

func TestClientStore_GetLimiterIsStablePerIP(t *testing.T) {
	store := NewClientStore(LimiterOptions{RequestsPerSecond: 5, Burst: 5})

	first := store.getLimiter("203.0.113.1")
	second := store.getLimiter("203.0.113.1")
	if first != second {
		t.Fatal("getLimiter returned a different *rate.Limiter for the same IP on a second call")
	}

	other := store.getLimiter("203.0.113.2")
	if other == first {
		t.Fatal("getLimiter returned the same *rate.Limiter for two different IPs")
	}
}

func TestClientStore_GetShardIsDeterministic(t *testing.T) {
	store := NewClientStore(LimiterOptions{NumShards: 8})

	first := store.getShard("203.0.113.1")
	second := store.getShard("203.0.113.1")
	if first != second {
		t.Fatal("getShard returned a different shard for the same IP on a second call")
	}
}

func TestNewClientStore_DefaultsNumShards(t *testing.T) {
	store := NewClientStore(LimiterOptions{}) // NumShards left unset (0)
	if store.numShards != 64 {
		t.Fatalf("numShards = %d, want the documented default of 64", store.numShards)
	}
	if len(store.shards) != 64 {
		t.Fatalf("len(shards) = %d, want 64", len(store.shards))
	}
}

func TestClientStore_CleanupEvictsIdleEntries(t *testing.T) {
	store := NewClientStore(LimiterOptions{RequestsPerSecond: 5, Burst: 5, ClientTTL: 10 * time.Millisecond})

	store.getLimiter("203.0.113.1") // creates the entry

	sh := store.getShard("203.0.113.1")
	sh.mu.RLock()
	_, exists := sh.clients["203.0.113.1"]
	sh.mu.RUnlock()
	if !exists {
		t.Fatal("test setup invariant broken: entry should exist right after getLimiter")
	}

	time.Sleep(20 * time.Millisecond) // let the entry age past ClientTTL

	for i := 0; i < store.numShards; i++ {
		store.cleanupShard(i)
	}

	sh.mu.RLock()
	_, stillExists := sh.clients["203.0.113.1"]
	sh.mu.RUnlock()
	if stillExists {
		t.Fatal("cleanupShard did not evict an entry idle past ClientTTL")
	}
}
