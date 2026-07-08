package ctxmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ReadOnlyContext manages a set of files loaded for agent reference that cannot be edited.
// Files are tracked with token budgets and can be auto-refreshed when they change on disk.
type ReadOnlyContext struct {
	Files          map[string]*ContextFile
	Patterns       []string
	MaxTokenBudget int
	mu             sync.RWMutex
}

// ContextFile represents a single read-only context file loaded into the agent's context.
type ContextFile struct {
	Path          string
	Content       string
	TokenCount    int
	AddedAt       time.Time
	LastRefreshed time.Time
	AutoRefresh   bool
	Pinned        bool
}

// ContextFileOption configures how a file is added to the read-only context.
type ContextFileOption func(*ContextFile)

// WithPinned marks the file as pinned so it is never evicted.
func WithPinned() ContextFileOption {
	return func(cf *ContextFile) {
		cf.Pinned = true
	}
}

// WithAutoRefresh marks the file to be re-read on each turn if it changes on disk.
func WithAutoRefresh() ContextFileOption {
	return func(cf *ContextFile) {
		cf.AutoRefresh = true
	}
}

// ContextStats provides statistics about the read-only context state.
type ContextStats struct {
	TotalFiles  int
	TotalTokens int
	BudgetUsed  float64
	PinnedCount int
}

// NewReadOnlyContext creates a new ReadOnlyContext with the given token budget.
func NewReadOnlyContext(maxBudget int) *ReadOnlyContext {
	return &ReadOnlyContext{
		Files:          make(map[string]*ContextFile),
		Patterns:       make([]string, 0),
		MaxTokenBudget: maxBudget,
	}
}

// AddFile reads a file from disk and adds it to the read-only context.
// Returns an error if the file cannot be read or would exceed the token budget.
func (rc *ReadOnlyContext) AddFile(path string, opts ...ContextFileOption) error {
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("readonly context: cannot read %s: %w", cleanPath, err)
	}

	content := string(data)
	tokens := TokenEstimate(content)

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Check budget (exclude existing entry for this path if re-adding)
	currentUsage := 0
	for p, f := range rc.Files {
		if p != cleanPath {
			currentUsage += f.TokenCount
		}
	}
	if currentUsage+tokens > rc.MaxTokenBudget {
		return fmt.Errorf("readonly context: adding %s (%d tokens) would exceed budget (%d/%d used)",
			cleanPath, tokens, currentUsage, rc.MaxTokenBudget)
	}

	now := time.Now()
	cf := &ContextFile{
		Path:          cleanPath,
		Content:       content,
		TokenCount:    tokens,
		AddedAt:       now,
		LastRefreshed: now,
	}

	for _, opt := range opts {
		opt(cf)
	}

	rc.Files[cleanPath] = cf
	return nil
}

// AddPattern adds a glob pattern and immediately resolves matching files.
// Files that match the pattern are added to the read-only context.
func (rc *ReadOnlyContext) AddPattern(pattern string) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("readonly context: invalid pattern %q: %w", pattern, err)
	}

	rc.mu.Lock()
	rc.Patterns = append(rc.Patterns, pattern)
	rc.mu.Unlock()

	var addErrors []string
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil || info.IsDir() {
			continue
		}
		if err := rc.AddFile(match); err != nil {
			addErrors = append(addErrors, err.Error())
		}
	}

	if len(addErrors) > 0 {
		return fmt.Errorf("readonly context: pattern %q had errors: %s", pattern, strings.Join(addErrors, "; "))
	}
	return nil
}

// RemoveFile removes a file from the read-only context.
func (rc *ReadOnlyContext) RemoveFile(path string) {
	cleanPath := filepath.Clean(path)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	delete(rc.Files, cleanPath)
}

// IsReadOnly checks if a path is in the read-only context.
// Used by Edit/Write tools to block modifications to context files.
func (rc *ReadOnlyContext) IsReadOnly(path string) bool {
	cleanPath := filepath.Clean(path)
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	_, exists := rc.Files[cleanPath]
	return exists
}

// RefreshStale re-reads files that have been modified on disk since last refresh.
// Only refreshes files with AutoRefresh=true.
func (rc *ReadOnlyContext) RefreshStale() error {
	rc.mu.RLock()
	var toRefresh []string
	for path, cf := range rc.Files {
		if !cf.AutoRefresh {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().After(cf.LastRefreshed) {
			toRefresh = append(toRefresh, path)
		}
	}
	rc.mu.RUnlock()

	var refreshErrors []string
	for _, path := range toRefresh {
		data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if err != nil {
			refreshErrors = append(refreshErrors, fmt.Sprintf("%s: %v", path, err))
			continue
		}

		content := string(data)
		tokens := TokenEstimate(content)

		rc.mu.Lock()
		if cf, exists := rc.Files[path]; exists {
			cf.Content = content
			cf.TokenCount = tokens
			cf.LastRefreshed = time.Now()
		}
		rc.mu.Unlock()
	}

	if len(refreshErrors) > 0 {
		return fmt.Errorf("readonly context: refresh errors: %s", strings.Join(refreshErrors, "; "))
	}
	return nil
}

// BuildContextBlock formats all context files for system prompt injection.
func (rc *ReadOnlyContext) BuildContextBlock() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if len(rc.Files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Read-Only Context Files (do NOT modify these)\n")

	// Sort paths for deterministic output
	paths := make([]string, 0, len(rc.Files))
	for p := range rc.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		cf := rc.Files[path]
		basename := filepath.Base(path)
		sb.WriteString("\n### ")
		sb.WriteString(basename)
		sb.WriteString("\n```\n")
		sb.WriteString(cf.Content)
		if !strings.HasSuffix(cf.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}

	return sb.String()
}

// Evict removes unpinned files by LRU (oldest AddedAt first) until within budget.
// Returns the list of evicted file paths.
func (rc *ReadOnlyContext) Evict() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	totalTokens := 0
	for _, cf := range rc.Files {
		totalTokens += cf.TokenCount
	}

	if totalTokens <= rc.MaxTokenBudget {
		return nil
	}

	// Collect unpinned files sorted by AddedAt (oldest first)
	type candidate struct {
		path    string
		addedAt time.Time
		tokens  int
	}
	var candidates []candidate
	for path, cf := range rc.Files {
		if !cf.Pinned {
			candidates = append(candidates, candidate{
				path:    path,
				addedAt: cf.AddedAt,
				tokens:  cf.TokenCount,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].addedAt.Before(candidates[j].addedAt)
	})

	var evicted []string
	for _, c := range candidates {
		if totalTokens <= rc.MaxTokenBudget {
			break
		}
		delete(rc.Files, c.path)
		totalTokens -= c.tokens
		evicted = append(evicted, c.path)
	}

	return evicted
}

// SuggestFiles returns a list of commonly useful context files that exist in the given directory.
func SuggestFiles(projectDir string) []string {
	suggestions := []string{
		"go.mod",
		"package.json",
		"Cargo.toml",
		"README.md",
		"ARCHITECTURE.md",
		"AGENTS.md",
		".env.example",
		"Makefile",
		"Dockerfile",
	}

	var found []string
	for _, name := range suggestions {
		full := filepath.Join(projectDir, name)
		if _, err := os.Stat(full); err == nil {
			found = append(found, full)
		}
	}
	return found
}

// TokenEstimate provides a quick token count approximation: len(content) / 4.
func TokenEstimate(content string) int {
	n := len(content) / 4
	if n == 0 && len(content) > 0 {
		return 1
	}
	return n
}

// Stats returns statistics about the current read-only context state.
func (rc *ReadOnlyContext) Stats() ContextStats {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	stats := ContextStats{
		TotalFiles: len(rc.Files),
	}

	for _, cf := range rc.Files {
		stats.TotalTokens += cf.TokenCount
		if cf.Pinned {
			stats.PinnedCount++
		}
	}

	if rc.MaxTokenBudget > 0 {
		stats.BudgetUsed = float64(stats.TotalTokens) / float64(rc.MaxTokenBudget) * 100.0
	}

	return stats
}
