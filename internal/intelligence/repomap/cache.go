// cache.go implements the in-process LRU symbol cache keyed
// by (path, modtime). It is consulted by parseFileSymbols before re-parsing
// and is cleared on process exit; for a persistent cache, use IncrementalMap.
package repomap

import (
	"container/list"
	"os"
	"sync"
	"time"
)

// defaultMaxSymbolCacheEntries is the default maximum number of symbol cache entries.
const defaultMaxSymbolCacheEntries = 5000

// cacheEntry holds cached symbols for a file with the file's mod time.
type cacheEntry struct {
	modTime time.Time
	symbols []Symbol
}

// lruCacheEntry wraps a cacheEntry with an LRU list element for O(1) eviction.
type lruCacheEntry struct {
	key   string
	entry cacheEntry
}

// symbolCacheData holds the LRU-managed symbol cache state.
type symbolCacheData struct {
	entries    map[string]*list.Element
	order      *list.List
	maxEntries int
}

var (
	cacheMu     sync.RWMutex
	symbolCache = &symbolCacheData{
		entries:    make(map[string]*list.Element),
		order:      list.New(),
		maxEntries: defaultMaxSymbolCacheEntries,
	}
)

// cacheGet returns cached symbols for path if the file hasn't been modified
// since the cache was populated. Promotes the entry on access.
func cacheGet(path string) ([]Symbol, bool) {
	cacheMu.Lock()
	elem, ok := symbolCache.entries[path]
	if !ok {
		cacheMu.Unlock()
		return nil, false
	}
	// Promote to front (most recently used)
	symbolCache.order.MoveToFront(elem)
	lru, _ := elem.Value.(*lruCacheEntry)
	cacheMu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if info.ModTime().After(lru.entry.modTime) {
		return nil, false // file was modified, cache stale
	}
	return lru.entry.symbols, true
}

// cachePut stores symbols for a file in the cache. Evicts the least recently
// used entry if the cache exceeds its maximum size.
func cachePut(path string, symbols []Symbol) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// If entry already exists, update and promote
	if elem, ok := symbolCache.entries[path]; ok {
		symbolCache.order.MoveToFront(elem)
		lru, _ := elem.Value.(*lruCacheEntry)
		lru.entry = cacheEntry{modTime: info.ModTime(), symbols: symbols}
		return
	}

	// Add new entry
	lru := &lruCacheEntry{
		key: path,
		entry: cacheEntry{
			modTime: info.ModTime(),
			symbols: symbols,
		},
	}
	elem := symbolCache.order.PushFront(lru)
	symbolCache.entries[path] = elem

	// Evict oldest if over capacity
	if symbolCache.order.Len() > symbolCache.maxEntries {
		oldest := symbolCache.order.Back()
		if oldest != nil {
			symbolCache.order.Remove(oldest)
			evicted, _ := oldest.Value.(*lruCacheEntry)
			delete(symbolCache.entries, evicted.key)
		}
	}
}

// CacheClear removes all entries from the symbol cache.
func CacheClear() {
	cacheMu.Lock()
	symbolCache.entries = make(map[string]*list.Element)
	symbolCache.order.Init()
	cacheMu.Unlock()
}

// CacheSize returns the number of entries in the cache.
func CacheSize() int {
	cacheMu.RLock()
	n := len(symbolCache.entries)
	cacheMu.RUnlock()
	return n
}
