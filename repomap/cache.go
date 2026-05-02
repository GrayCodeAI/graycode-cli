package repomap

import (
	"os"
	"sync"
	"time"
)

// cacheEntry holds cached symbols for a file with the file's mod time.
type cacheEntry struct {
	modTime time.Time
	symbols []Symbol
}

var (
	cacheMu    sync.RWMutex
	symbolCache = make(map[string]cacheEntry)
)

// cacheGet returns cached symbols for path if the file hasn't been modified
// since the cache was populated. Returns (symbols, ok).
func cacheGet(path string) ([]Symbol, bool) {
	cacheMu.RLock()
	entry, ok := symbolCache[path]
	cacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if info.ModTime().After(entry.modTime) {
		return nil, false // file was modified, cache stale
	}
	return entry.symbols, true
}

// cachePut stores symbols for a file in the cache.
func cachePut(path string, symbols []Symbol) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	cacheMu.Lock()
	symbolCache[path] = cacheEntry{
		modTime: info.ModTime(),
		symbols: symbols,
	}
	cacheMu.Unlock()
}

// CacheClear removes all entries from the symbol cache.
func CacheClear() {
	cacheMu.Lock()
	symbolCache = make(map[string]cacheEntry)
	cacheMu.Unlock()
}

// CacheSize returns the number of entries in the cache.
func CacheSize() int {
	cacheMu.RLock()
	n := len(symbolCache)
	cacheMu.RUnlock()
	return n
}
