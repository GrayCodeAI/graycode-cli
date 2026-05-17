package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// IncrementalMap maintains a cached symbol index that only reprocesses changed files.
// It stores file hashes (SHA-256 of content) in a cache file (.hawk/repomap-cache.json).
// On regeneration, only files whose hash changed are re-parsed. Symbols from changed
// files are merged into the existing map, and symbols from deleted files are removed.
type IncrementalMap struct {
	cacheFile string
	cache     map[string]FileCache // path -> {hash, symbols, mtime}
	mu        sync.Mutex
}

// FileCache holds the cached metadata for a single file.
type FileCache struct {
	Hash    string   `json:"hash"`
	Mtime   int64    `json:"mtime"`
	Symbols []string `json:"symbols"`
}

// NewIncrementalMap loads or creates a repomap cache.
// cacheDir is the directory where the cache file will be stored (typically ".hawk").
func NewIncrementalMap(cacheDir string) (*IncrementalMap, error) {
	cacheFile := filepath.Join(cacheDir, "repomap-cache.json")

	im := &IncrementalMap{
		cacheFile: cacheFile,
		cache:     make(map[string]FileCache),
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return im, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &im.cache); err != nil {
		// Corrupted cache -- start fresh
		im.cache = make(map[string]FileCache)
		return im, nil
	}

	return im, nil
}

// Update scans the directory and only reprocesses changed files.
// Returns the list of files that were re-indexed.
// Uses fast change detection: checks mtime first, only hashes if mtime differs.
func (im *IncrementalMap) Update(rootDir string) (changed []string, err error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}

	// Track which files still exist on disk
	seenPaths := make(map[string]bool)

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if ignoreSet[base] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if !isSupportedExt(ext) {
			return nil
		}

		relPath, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			relPath = path
		}

		seenPaths[relPath] = true

		mtime := info.ModTime().UnixNano()

		// Fast path: if mtime hasn't changed, skip this file entirely
		if cached, ok := im.cache[relPath]; ok && cached.Mtime == mtime {
			return nil
		}

		// Mtime changed (or new file): compute hash
		contentHash, hashErr := computeContentHash(path)
		if hashErr != nil {
			return nil // skip unreadable files
		}

		// If hash matches the cached hash, just update the mtime
		if cached, ok := im.cache[relPath]; ok && cached.Hash == contentHash {
			cached.Mtime = mtime
			im.cache[relPath] = cached
			return nil
		}

		// File content actually changed (or is new): re-parse symbols
		symbols := parseFileSymbols(path)
		symbolNames := make([]string, 0, len(symbols))
		for _, sym := range symbols {
			symbolNames = append(symbolNames, sym.Kind+" "+sym.Name)
		}

		im.cache[relPath] = FileCache{
			Hash:    contentHash,
			Mtime:   mtime,
			Symbols: symbolNames,
		}
		changed = append(changed, relPath)

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Remove symbols from deleted files
	for cachedPath := range im.cache {
		if !seenPaths[cachedPath] {
			delete(im.cache, cachedPath)
			changed = append(changed, cachedPath)
		}
	}

	return changed, nil
}

// Symbols returns all cached symbols for a file.
func (im *IncrementalMap) Symbols(path string) []string {
	im.mu.Lock()
	defer im.mu.Unlock()

	if cached, ok := im.cache[path]; ok {
		result := make([]string, len(cached.Symbols))
		copy(result, cached.Symbols)
		return result
	}
	return nil
}

// AllSymbols returns every symbol across all cached files.
func (im *IncrementalMap) AllSymbols() map[string][]string {
	im.mu.Lock()
	defer im.mu.Unlock()

	result := make(map[string][]string, len(im.cache))
	for path, cached := range im.cache {
		syms := make([]string, len(cached.Symbols))
		copy(syms, cached.Symbols)
		result[path] = syms
	}
	return result
}

// Save persists the cache to disk.
func (im *IncrementalMap) Save() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	dir := filepath.Dir(im.cacheFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(im.cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(im.cacheFile, data, 0o644)
}

// computeContentHash returns the SHA-256 hex digest of a file's contents.
func computeContentHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
