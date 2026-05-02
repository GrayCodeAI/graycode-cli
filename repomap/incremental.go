package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

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

// IncrementalReindex walks dir, chunks supported source files, and stores them
// via the indexer. Files whose hash matches the stored hash are skipped. Files
// that have been removed from disk are cleared from the index.
func IncrementalReindex(dir string, ignore []string, indexer CodeIndexer) (added, skipped, removed int, err error) {
	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}
	for _, p := range ignore {
		ignoreSet[p] = true
	}

	// Track which paths we see on disk
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

		// Compute hash and compare with stored hash
		fileHash, hashErr := ComputeFileHash(path)
		if hashErr != nil {
			return nil // skip unreadable files
		}

		storedHash, hashErr := indexer.GetFileHash(relPath)
		if hashErr != nil {
			return nil // skip on error
		}

		if storedHash == fileHash {
			skipped++
			return nil
		}

		// File changed or new: clear old chunks and re-index
		if err := indexer.ClearFileChunks(relPath); err != nil {
			return nil // skip on error
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		opts := tok.ChunkOptions{
			MaxTokens: 500,
			MinTokens: 50,
			Language:  lang,
		}
		chunks := tok.ChunkCode(string(data), opts)

		for i, chunk := range chunks {
			chunkID := fmt.Sprintf("%s:%d", relPath, i)
			if idxErr := indexer.IndexCodeChunk(
				relPath,
				chunk.Content,
				chunk.Symbol,
				lang,
				chunk.StartLine,
				chunk.EndLine,
				chunk.Tokens,
				fileHash,
			); idxErr != nil {
				_ = chunkID // used for context, not needed as arg
				return nil
			}
		}
		added++
		return nil
	})
	if err != nil {
		return added, skipped, removed, fmt.Errorf("walk: %w", err)
	}

	// Remove indexed paths that no longer exist on disk
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
