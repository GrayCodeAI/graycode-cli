package engine

import (
	"sync"
	"time"
)

// SourceRoot tracks a directory the agent has explored.
type SourceRoot struct {
	Path       string
	ExploredAt time.Time
	FileCount  int
}

// SourceRoots tracks which directories the agent has explored to avoid re-scanning.
type SourceRoots struct {
	mu    sync.Mutex
	roots map[string]*SourceRoot
}

// NewSourceRoots creates a source root tracker.
func NewSourceRoots() *SourceRoots {
	return &SourceRoots{roots: make(map[string]*SourceRoot)}
}

// Mark records that a directory has been explored.
func (sr *SourceRoots) Mark(path string, fileCount int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.roots[path] = &SourceRoot{
		Path:       path,
		ExploredAt: time.Now(),
		FileCount:  fileCount,
	}
}

// IsExplored reports whether a directory has been explored.
func (sr *SourceRoots) IsExplored(path string) bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	_, ok := sr.roots[path]
	return ok
}

// List returns all explored roots.
func (sr *SourceRoots) List() []*SourceRoot {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	var list []*SourceRoot
	for _, r := range sr.roots {
		list = append(list, r)
	}
	return list
}

// Stale returns roots explored more than maxAge ago.
func (sr *SourceRoots) Stale(maxAge time.Duration) []*SourceRoot {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	var stale []*SourceRoot
	cutoff := time.Now().Add(-maxAge)
	for _, r := range sr.roots {
		if r.ExploredAt.Before(cutoff) {
			stale = append(stale, r)
		}
	}
	return stale
}

// Invalidate removes a root (e.g., after files changed).
func (sr *SourceRoots) Invalidate(path string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	delete(sr.roots, path)
}
