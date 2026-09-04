// Package context provides hierarchical context discovery for graycode.
// It implements walk-up AGENTS.md discovery (pi-nested-agents-md pattern):
// when an agent reads a file, the discoverer traverses upward collecting
// convention files at each directory level with session-scoped deduplication.
package context

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/hooks"
)

const (
	defaultMaxFileKB  = 32
	defaultMaxTotalKB = 128
)

// DefaultConventionFiles lists the files to discover at each directory level.
var DefaultConventionFiles = []string{
	"AGENTS.md", "GRAYCODE.md", "CLAUDE.md", "CONTEXT.md",
}

// InjectionCache tracks which files have already been injected this session.
type InjectionCache struct {
	mu      sync.Mutex
	entries map[string]string // path -> content hash
}

// NewInjectionCache creates a new injection cache.
func NewInjectionCache() *InjectionCache {
	return &InjectionCache{entries: make(map[string]string)}
}

// IsSeen returns true if the file at path with the given content hash
// has already been injected in this session.
func (c *InjectionCache) IsSeen(path, hash string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[path] == hash
}

// Mark records a file as injected.
func (c *InjectionCache) Mark(path, hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[path] = hash
}

// Clear removes all entries from the cache.
func (c *InjectionCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]string)
}

// Len returns the number of cached entries.
func (c *InjectionCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// WalkUpDiscoverer discovers convention files by walking up from a file's directory
// to the project root. It hooks into file read events and injects discovered
// conventions as additional context.
type WalkUpDiscoverer struct {
	projectRoot string
	cache       *InjectionCache
	maxFileKB   int
	maxTotalKB  int
	fileNames   []string
	pending     []string // accumulated convention content
	mu          sync.Mutex
}

// NewWalkUpDiscoverer creates a discoverer for the given project root.
func NewWalkUpDiscoverer(projectRoot string) *WalkUpDiscoverer {
	return &WalkUpDiscoverer{
		projectRoot: projectRoot,
		cache:       NewInjectionCache(),
		maxFileKB:   defaultMaxFileKB,
		maxTotalKB:  defaultMaxTotalKB,
		fileNames:   DefaultConventionFiles,
	}
}

// WithMaxFileKB sets the max size per convention file (default 32KB).
func (d *WalkUpDiscoverer) WithMaxFileKB(kb int) *WalkUpDiscoverer {
	d.maxFileKB = kb
	return d
}

// WithMaxTotalKB sets the max total injection size per event (default 128KB).
func (d *WalkUpDiscoverer) WithMaxTotalKB(kb int) *WalkUpDiscoverer {
	d.maxTotalKB = kb
	return d
}

// WithFileNames sets the convention file names to look for.
func (d *WalkUpDiscoverer) WithFileNames(names []string) *WalkUpDiscoverer {
	d.fileNames = names
	return d
}

// Discover walks up from filePath to projectRoot, collecting undiscovered convention files.
// Returns the content of newly discovered files (already deduplicated and truncated).
func (d *WalkUpDiscoverer) Discover(filePath string) []ConventionFile {
	dir := filepath.Dir(filePath)
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil
		}
		dir = abs
	}

	var discovered []ConventionFile
	totalBytes := 0
	maxTotal := d.maxTotalKB * 1024
	maxFile := d.maxFileKB * 1024

	// Walk from file's directory up to project root
	for {
		for _, name := range d.fileNames {
			candidate := filepath.Join(dir, name)
			data, err := os.ReadFile(candidate) // #nosec G304 -- candidate is built from a fixed set of convention file names joined with the walked-up directory, not external input
			if err != nil {
				continue // file doesn't exist at this level
			}

			absCandidate, _ := filepath.Abs(candidate)
			hash := fmt.Sprintf("%x", sha256.Sum256(data))

			// Skip if already injected this session
			if d.cache.IsSeen(absCandidate, hash) {
				continue
			}

			// Truncate if too large
			content := string(data)
			if len(content) > maxFile {
				content = content[:maxFile] + "\n... (truncated)"
			}

			// Check total budget
			if totalBytes+len(content) > maxTotal {
				break
			}

			totalBytes += len(content)
			d.cache.Mark(absCandidate, hash)
			discovered = append(discovered, ConventionFile{
				Path:     absCandidate,
				Content:  content,
				Level:    dirLevel(d.projectRoot, dir),
				FileName: name,
			})
		}

		// Stop if we've reached the project root
		if dir == d.projectRoot || dir == filepath.Dir(dir) {
			break
		}
		dir = filepath.Dir(dir)
	}

	return discovered
}

// ConventionFile represents a discovered convention file.
type ConventionFile struct {
	Path     string
	Content  string
	Level    int    // 0 = project root, 1 = one level down, etc.
	FileName string // e.g., "AGENTS.md"
}

// HandlePostTool is a hook handler for EventPostTool that discovers conventions
// when a file is read.
func (d *WalkUpDiscoverer) HandlePostTool(ctx interface{}, envelope hooks.EventEnvelope) error {
	// Only trigger on file read events
	toolName, _ := envelope.Payload["tool"].(string)
	if toolName != "Read" && toolName != "file_read" && toolName != "FileRead" {
		return nil
	}

	// Extract file path from payload
	path := extractFilePath(envelope.Payload)
	if path == "" {
		return nil
	}

	discovered := d.Discover(path)
	if len(discovered) == 0 {
		return nil
	}

	d.mu.Lock()
	for _, cf := range discovered {
		d.pending = append(d.pending, formatConvention(cf))
	}
	d.mu.Unlock()

	slog.Debug("walkup: discovered conventions", "count", len(discovered), "file", path)
	return nil
}

// HandleCompact is a hook handler for EventPostCompact that clears the cache.
func (d *WalkUpDiscoverer) HandleCompact(ctx interface{}, envelope hooks.EventEnvelope) error {
	d.cache.Clear()
	d.mu.Lock()
	d.pending = nil
	d.mu.Unlock()
	slog.Debug("walkup: cache cleared after compaction")
	return nil
}

// FlushPending returns accumulated convention content and clears the pending buffer.
func (d *WalkUpDiscoverer) FlushPending() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.pending) == 0 {
		return ""
	}
	result := "--- DISCOVERED CONVENTIONS ---\n" + strings.Join(d.pending, "\n---\n") + "\n--- END CONVENTIONS ---"
	d.pending = nil
	return result
}

// Cache returns the injection cache (for testing).
func (d *WalkUpDiscoverer) Cache() *InjectionCache {
	return d.cache
}

func extractFilePath(payload map[string]interface{}) string {
	for _, key := range []string{"path", "file_path", "filePath", "file"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func formatConvention(cf ConventionFile) string {
	return fmt.Sprintf("[%s] (%s)\n%s", cf.FileName, cf.Path, cf.Content)
}

func dirLevel(root, dir string) int {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return 0
	}
	if rel == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}
