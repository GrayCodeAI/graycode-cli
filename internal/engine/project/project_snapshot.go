package project

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultProjectSnapshotTTL is the default time-to-live for a cached project snapshot.
// Matches herm's 10s TTL to avoid redundant shell commands when spawning multiple
// sub-agents against an unchanged repo.
const DefaultProjectSnapshotTTL = 10 * time.Second

// projectSnapshotCmdTimeout is the per-command timeout for gathering snapshot data.
const projectSnapshotCmdTimeout = 2 * time.Second

// ProjectSnapshot holds a point-in-time view of project state gathered via shell
// commands. Inspired by herm's projectSnapshot pattern.
type ProjectSnapshot struct {
	DirectoryListing string    // ls -1 output of the project root
	RecentCommits    string    // git log --oneline -10
	GitStatus        string    // git status --short
	GatheredAt       time.Time // when this snapshot was created
}

// ForExploreMode returns a copy of the snapshot with GitStatus cleared.
// This is useful for read-only agents that don't need working tree state,
// saving tokens in the prompt.
func (s *ProjectSnapshot) ForExploreMode() *ProjectSnapshot {
	if s == nil {
		return nil
	}
	return &ProjectSnapshot{
		DirectoryListing: s.DirectoryListing,
		RecentCommits:    s.RecentCommits,
		GitStatus:        "",
		GatheredAt:       s.GatheredAt,
	}
}

// ProjectSnapshotCache caches a ProjectSnapshot for a given directory, refreshing
// it only after the TTL expires. This avoids redundant shell commands when multiple
// sub-agents spawn in rapid succession against an unchanged repo.
type ProjectSnapshotCache struct {
	mu       sync.Mutex
	snapshot *ProjectSnapshot
	ttl      time.Duration
	dir      string
}

// NewProjectSnapshotCache creates a new ProjectSnapshotCache for the given directory.
// If ttl is zero, DefaultProjectSnapshotTTL is used.
func NewProjectSnapshotCache(dir string, ttl time.Duration) *ProjectSnapshotCache {
	if ttl == 0 {
		ttl = DefaultProjectSnapshotTTL
	}
	return &ProjectSnapshotCache{
		dir: dir,
		ttl: ttl,
	}
}

// Get returns a cached project snapshot if it's still valid, or gathers a fresh
// one by running shell commands. The provided context controls overall cancellation
// but individual commands have a 2s timeout.
func (c *ProjectSnapshotCache) Get(ctx context.Context) *ProjectSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.snapshot != nil && time.Since(c.snapshot.GatheredAt) < c.ttl {
		return c.snapshot
	}

	snap := gatherProjectSnapshot(ctx, c.dir)
	c.snapshot = snap
	return snap
}

// Invalidate forces the next Get call to gather a fresh snapshot.
func (c *ProjectSnapshotCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot = nil
}

// gatherProjectSnapshot runs shell commands concurrently to build a ProjectSnapshot.
func gatherProjectSnapshot(ctx context.Context, dir string) *ProjectSnapshot {
	type result struct {
		field string
		value string
	}

	ch := make(chan result, 3)

	// ls -1 of project root
	go func() {
		cmdCtx, cancel := context.WithTimeout(ctx, projectSnapshotCmdTimeout)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, "ls", "-1")
		cmd.Dir = dir
		out, err := cmd.Output()
		val := ""
		if err == nil {
			val = strings.TrimSpace(string(out))
		}
		ch <- result{"ls", val}
	}()

	// git log --oneline -10
	go func() {
		cmdCtx, cancel := context.WithTimeout(ctx, projectSnapshotCmdTimeout)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, "git", "log", "--oneline", "-10")
		cmd.Dir = dir
		out, err := cmd.Output()
		val := ""
		if err == nil {
			val = strings.TrimSpace(string(out))
		}
		ch <- result{"log", val}
	}()

	// git status --short
	go func() {
		cmdCtx, cancel := context.WithTimeout(ctx, projectSnapshotCmdTimeout)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, "git", "status", "--short")
		cmd.Dir = dir
		out, err := cmd.Output()
		val := ""
		if err == nil {
			val = strings.TrimSpace(string(out))
		}
		ch <- result{"status", val}
	}()

	snap := &ProjectSnapshot{
		GatheredAt: time.Now(),
	}

	for i := 0; i < 3; i++ {
		r := <-ch
		switch r.field {
		case "ls":
			snap.DirectoryListing = r.value
		case "log":
			snap.RecentCommits = r.value
		case "status":
			snap.GitStatus = r.value
		}
	}

	return snap
}
