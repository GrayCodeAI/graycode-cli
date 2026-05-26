package engine

import (
	"container/list"
	"sync"
	"time"
)

// DefaultSnapshotTTL is the default time-to-live for snapshot cache entries.
const DefaultSnapshotTTL = 10 * time.Second

// defaultMaxSnapshotEntries is the default maximum number of snapshot cache entries.
const defaultMaxSnapshotEntries = 500

// cachedEntry holds a value and its expiry time.
type cachedEntry struct {
	value  string
	expiry time.Time
}

// lruSnapshotEntry wraps a cachedEntry with an LRU list element.
type lruSnapshotEntry struct {
	key   string
	entry cachedEntry
}

// SnapshotCache is a TTL-based cache for project snapshots with LRU eviction.
type SnapshotCache struct {
	entries    map[string]*list.Element
	order      *list.List
	mu         sync.RWMutex
	ttl        time.Duration
	maxEntries int
}

// NewSnapshotCache creates a new SnapshotCache with the given TTL.
// If ttl is zero, DefaultSnapshotTTL is used.
func NewSnapshotCache(ttl time.Duration) *SnapshotCache {
	if ttl == 0 {
		ttl = DefaultSnapshotTTL
	}
	return &SnapshotCache{
		entries:    make(map[string]*list.Element),
		order:      list.New(),
		ttl:        ttl,
		maxEntries: defaultMaxSnapshotEntries,
	}
}

// Get returns the cached value for the key if it exists and has not expired.
// Promotes the entry to most recently used on hit.
func (sc *SnapshotCache) Get(key string) (string, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	elem, ok := sc.entries[key]
	if !ok {
		return "", false
	}

	lru := elem.Value.(*lruSnapshotEntry)
	if time.Now().After(lru.entry.expiry) {
		// Expired: remove from cache
		sc.order.Remove(elem)
		delete(sc.entries, key)
		return "", false
	}

	// Promote to front (most recently used)
	sc.order.MoveToFront(elem)
	return lru.entry.value, true
}

// Set stores a value in the cache with the configured TTL.
// Evicts the least recently used entry if the cache exceeds its maximum size.
func (sc *SnapshotCache) Set(key, value string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// If entry already exists, update and promote
	if elem, ok := sc.entries[key]; ok {
		sc.order.MoveToFront(elem)
		lru := elem.Value.(*lruSnapshotEntry)
		lru.entry = cachedEntry{
			value:  value,
			expiry: time.Now().Add(sc.ttl),
		}
		return
	}

	// Add new entry
	lru := &lruSnapshotEntry{
		key: key,
		entry: cachedEntry{
			value:  value,
			expiry: time.Now().Add(sc.ttl),
		},
	}
	elem := sc.order.PushFront(lru)
	sc.entries[key] = elem

	// Evict oldest if over capacity
	if sc.order.Len() > sc.maxEntries {
		oldest := sc.order.Back()
		if oldest != nil {
			sc.order.Remove(oldest)
			evicted := oldest.Value.(*lruSnapshotEntry)
			delete(sc.entries, evicted.key)
		}
	}
}

// GetOrCompute returns the cached value for the key, or computes and caches it
// using the provided function if the value is missing or expired.
func (sc *SnapshotCache) GetOrCompute(key string, fn func() (string, error)) (string, error) {
	sc.mu.Lock()
	// Re-check under write lock to avoid duplicate computation.
	if elem, ok := sc.entries[key]; ok {
		lru := elem.Value.(*lruSnapshotEntry)
		if time.Now().Before(lru.entry.expiry) {
			val := lru.entry.value
			sc.order.MoveToFront(elem)
			sc.mu.Unlock()
			return val, nil
		}
		// Expired; remove before recomputing
		sc.order.Remove(elem)
		delete(sc.entries, key)
	}
	sc.mu.Unlock()

	val, err := fn()
	if err != nil {
		return "", err
	}

	sc.Set(key, val)
	return val, nil
}
