package engine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ProtectedPaths tracks file paths that are read-only within the session.
// Tools that write or edit files should check IsProtected before proceeding.
type ProtectedPaths struct {
	mu    sync.RWMutex
	paths map[string]bool
}

// NewProtectedPaths creates an empty ProtectedPaths set.
func NewProtectedPaths() *ProtectedPaths {
	return &ProtectedPaths{
		paths: make(map[string]bool),
	}
}

// Add marks a path as protected (read-only).
// The path is cleaned before storage for consistent lookups.
func (p *ProtectedPaths) Add(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paths[filepath.Clean(path)] = true
}

// Remove unmarks a path so it is no longer protected.
func (p *ProtectedPaths) Remove(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.paths, filepath.Clean(path))
}

// IsProtected returns true when path (or any ancestor directory) is protected.
func (p *ProtectedPaths) IsProtected(path string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clean := filepath.Clean(path)

	// Exact match
	if p.paths[clean] {
		return true
	}

	// Check whether any protected entry is an ancestor directory of path.
	for prot := range p.paths {
		if strings.HasPrefix(clean, prot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// List returns a sorted slice of all protected paths.
func (p *ProtectedPaths) List() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.paths))
	for path := range p.paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// Format returns a human-readable block suitable for system prompt injection.
func (p *ProtectedPaths) Format() string {
	paths := p.List()
	if len(paths) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following paths are READ-ONLY. Do NOT write to or edit them:\n")
	for _, path := range paths {
		fmt.Fprintf(&b, "  - %s\n", path)
	}
	return b.String()
}
