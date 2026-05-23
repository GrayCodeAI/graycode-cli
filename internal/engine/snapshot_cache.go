package engine

import (
	"sync"
	"time"
)

// DefaultSnapshotTTL is the default time-to-live for snapshot cache entries.
const DefaultSnapshotTTL = 10 * time.Second

// cachedEntry holds a value and its expiry time.
type cachedEntry struct {
	value  string
	expiry time.Time
}

// SnapshotCache is a TTL-based cache for project snapshots.
type SnapshotCache struct {
	entries map[string]cachedEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

// NewSnapshotCache creates a new SnapshotCache with the given TTL.
// If ttl is zero, DefaultSnapshotTTL is used.
func NewSnapshotCache(ttl time.Duration) *SnapshotCache {
	if ttl == 0 {
		ttl = DefaultSnapshotTTL
	}
	return &SnapshotCache{
		entries: make(map[string]cachedEntry),
		ttl:     ttl,
	}
}

// Get returns the cached value for the key if it exists and has not expired.
func (sc *SnapshotCache) Get(key string) (string, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	entry, ok := sc.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiry) {
		return "", false
	}
	return entry.value, true
}

// Set stores a value in the cache with the configured TTL.
func (sc *SnapshotCache) Set(key, value string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.entries[key] = cachedEntry{
		value:  value,
		expiry: time.Now().Add(sc.ttl),
	}
}

// GetOrCompute returns the cached value for the key, or computes and caches it
// using the provided function if the value is missing or expired.
func (sc *SnapshotCache) GetOrCompute(key string, fn func() (string, error)) (string, error) {
	sc.mu.Lock()
	// Re-check under write lock to avoid duplicate computation.
	entry, ok := sc.entries[key]
	if ok && time.Now().Before(entry.expiry) {
		val := entry.value
		sc.mu.Unlock()
		return val, nil
	}
	sc.mu.Unlock()

	val, err := fn()
	if err != nil {
		return "", err
	}

	sc.Set(key, val)
	return val, nil
}
