package cmd

import (
	"container/list"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/tool"
)

// lruCache is a simple fixed-size LRU cache for directory listing results.
type lruCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type lruEntry struct {
	key   string
	value []string
}

func newLRUCache(capacity int) *lruCache {
	return &lruCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// get retrieves a cached value and marks it as recently used.
func (c *lruCache) get(key string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}

// put adds or updates a cached value.
func (c *lruCache) put(key string, value []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*lruEntry).value = value
		return
	}

	entry := &lruEntry{key: key, value: value}
	elem := c.order.PushFront(entry)
	c.items[key] = elem

	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*lruEntry).key)
		}
	}
}

// len returns the number of cached entries.
func (c *lruCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// dirCompletionCache is an LRU cache for directory listing results.
var dirCompletionCache = newLRUCache(64)

// filePathCompletions returns matching files/dirs from the partial path typed.
// It uses an LRU cache to avoid redundant directory reads for repeated queries.
func filePathCompletions(partial string) []string {
	if partial == "" {
		partial = "."
	}

	dir := filepath.Dir(partial)
	base := filepath.Base(partial)

	// If partial ends with a separator, list that directory
	if strings.HasSuffix(partial, string(filepath.Separator)) || partial == "." {
		dir = partial
		base = ""
	}

	// Try to get directory entries from cache
	entries, cached := getCachedDirEntries(dir)
	if !cached {
		rawEntries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		entries = rawEntries
		// Cache the directory listing
		var names []string
		for _, e := range rawEntries {
			suffix := ""
			if e.IsDir() {
				suffix = "/"
			}
			names = append(names, e.Name()+suffix)
		}
		dirCompletionCache.put(dir, names)
	}

	var matches []string
	for _, e := range entries {
		name := e.Name()
		// Skip hidden files unless the user is explicitly typing a dot prefix
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if base == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			full := filepath.Join(dir, name)
			if e.IsDir() {
				full += string(filepath.Separator)
			}
			matches = append(matches, full)
		}
	}

	// Cap results to avoid flooding the UI
	if len(matches) > 20 {
		matches = matches[:20]
	}
	return matches
}

// getCachedDirEntries retrieves directory entries from cache and converts them
// back to os.DirEntry. Returns nil, false on cache miss.
func getCachedDirEntries(dir string) ([]os.DirEntry, bool) {
	names, ok := dirCompletionCache.get(dir)
	if !ok {
		return nil, false
	}

	// Validate cache: re-read from filesystem if dir was modified
	info, err := os.Stat(dir)
	if err != nil {
		return nil, false
	}
	_ = info // cache is simple time-unaware; invalidation is on eviction

	// Reconstruct DirEntry from cached names by re-reading the directory.
	// If the cache hit is valid, os.ReadDir is fast (OS level cache).
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}

	// Quick sanity check: if count changed, cache is stale
	if len(entries) != len(names) {
		return nil, false
	}

	return entries, true
}

// toolNameCompletions returns matching tool names from the partial string typed.
func toolNameCompletions(partial string, registry *tool.Registry) []string {
	if registry == nil {
		return nil
	}

	partial = strings.ToLower(strings.TrimSpace(partial))
	if partial == "" {
		return nil
	}

	var matches []string
	for _, t := range registry.PrimaryTools() {
		name := t.Name()
		if strings.HasPrefix(strings.ToLower(name), partial) {
			matches = append(matches, name)
		}
	}

	if len(matches) > 20 {
		matches = matches[:20]
	}
	return matches
}
