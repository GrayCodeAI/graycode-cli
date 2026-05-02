package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/GrayCodeAI/tok"
)

// CodeIndexer is the interface used by IncrementalReindex to store and query
// code chunks. The memory package's YaadBridge implements this interface.
type CodeIndexer interface {
	IndexCodeChunk(path, content, symbol, lang string, start, end, tokens int, hash string) error
	SearchCode(query string, limit int) ([]CodeSearchResult, error)
	GetFileHash(path string) (string, error)
	ClearFileChunks(path string) error
	ListIndexedPaths() ([]string, error)
}

// CodeSearchResult represents a code chunk returned by a search.
type CodeSearchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Symbol    string
	Score     float64
}

// ComputeFileHash returns the SHA-256 hex digest of a file's contents.
func ComputeFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// supportedCodeExts lists extensions for code-aware chunking.
var supportedCodeExts = map[string]string{
	".go":   "go",
	".py":   "python",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "javascript",
	".jsx":  "javascript",
	".rs":   "rust",
	".java": "java",
}

// fileWork represents a file to be processed during reindexing.
type fileWork struct {
	absPath string
	relPath string
	lang    string
}

// IncrementalReindex walks dir, chunks supported source files, and stores them
// via the indexer. Files whose hash matches the stored hash are skipped. Files
// that have been removed from disk are cleared from the index.
// File processing is parallelized across available CPUs.
func IncrementalReindex(dir string, ignore []string, indexer CodeIndexer) (added, skipped, removed int, err error) {
	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}
	for _, p := range ignore {
		ignoreSet[p] = true
	}

	// Phase 1: collect all candidate files (sequential walk)
	var files []fileWork
	seenPaths := make(map[string]bool)

	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
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
		lang, ok := supportedCodeExts[ext]
		if !ok {
			return nil
		}

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relPath = path
		}
		seenPaths[relPath] = true
		files = append(files, fileWork{absPath: path, relPath: relPath, lang: lang})
		return nil
	})
	if err != nil {
		return 0, 0, 0, fmt.Errorf("walk: %w", err)
	}

	// Phase 2: process files in parallel using a bounded goroutine pool
	var mu sync.Mutex
	var wg sync.WaitGroup

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	sem := make(chan struct{}, numWorkers)

	for _, fw := range files {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore slot
		go func(fw fileWork) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore slot

			// Compute hash and compare with stored hash
			fileHash, hashErr := ComputeFileHash(fw.absPath)
			if hashErr != nil {
				return // skip unreadable files
			}

			storedHash, hashErr := indexer.GetFileHash(fw.relPath)
			if hashErr != nil {
				return // skip on error
			}

			if storedHash == fileHash {
				mu.Lock()
				skipped++
				mu.Unlock()
				return
			}

			// File changed or new: clear old chunks and re-index
			if clearErr := indexer.ClearFileChunks(fw.relPath); clearErr != nil {
				return // skip on error
			}

			data, readErr := os.ReadFile(fw.absPath)
			if readErr != nil {
				return
			}

			opts := tok.ChunkOptions{
				MaxTokens: 500,
				MinTokens: 50,
				Language:  fw.lang,
			}
			chunks := tok.ChunkCode(string(data), opts)

			for i, chunk := range chunks {
				chunkID := fmt.Sprintf("%s:%d", fw.relPath, i)
				if idxErr := indexer.IndexCodeChunk(
					fw.relPath,
					chunk.Content,
					chunk.Symbol,
					fw.lang,
					chunk.StartLine,
					chunk.EndLine,
					chunk.Tokens,
					fileHash,
				); idxErr != nil {
					_ = chunkID
					return
				}
			}

			mu.Lock()
			added++
			mu.Unlock()
		}(fw)
	}
	wg.Wait()

	// Phase 3: remove indexed paths that no longer exist on disk
	indexedPaths, listErr := indexer.ListIndexedPaths()
	if listErr == nil {
		for _, p := range indexedPaths {
			if !seenPaths[p] {
				if clearErr := indexer.ClearFileChunks(p); clearErr == nil {
					removed++
				}
			}
		}
	}

	return added, skipped, removed, nil
}
