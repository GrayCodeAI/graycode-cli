package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProactiveContext provides file-aware, topic-aware memory injection.
// Instead of waiting for explicit recall calls, it tracks which files
// the agent is working on and proactively surfaces relevant memories.
type ProactiveContext struct {
	bridge       *YaadBridge
	activeFiles  map[string]time.Time
	injectedKeys map[string]bool // track what we've already injected to avoid duplicates
	mu           sync.Mutex
	maxAge       time.Duration
}

// NewProactiveContext creates a proactive context injector.
func NewProactiveContext(bridge *YaadBridge) *ProactiveContext {
	return &ProactiveContext{
		bridge:       bridge,
		activeFiles:  make(map[string]time.Time),
		injectedKeys: make(map[string]bool),
		maxAge:       30 * time.Minute,
	}
}

// TrackFile records that the agent is working on a specific file.
func (pc *ProactiveContext) TrackFile(path string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.activeFiles[path] = time.Now()
	pc.pruneStale()
}

// TrackFiles records multiple file interactions at once.
func (pc *ProactiveContext) TrackFiles(paths []string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	now := time.Now()
	for _, p := range paths {
		pc.activeFiles[p] = now
	}
	pc.pruneStale()
}

func (pc *ProactiveContext) pruneStale() {
	cutoff := time.Now().Add(-pc.maxAge)
	for path, ts := range pc.activeFiles {
		if ts.Before(cutoff) {
			delete(pc.activeFiles, path)
		}
	}
}

// ContextForFile returns relevant memories for a specific file path.
func (pc *ProactiveContext) ContextForFile(path string) string {
	if !pc.bridge.Ready() || path == "" {
		return ""
	}

	pc.TrackFile(path)

	// Search yaad for memories related to this file
	basename := filepath.Base(path)
	dir := filepath.Dir(path)
	dirBase := filepath.Base(dir)

	queries := []string{basename, dirBase}
	if ext := filepath.Ext(path); ext != "" {
		queries = append(queries, strings.TrimPrefix(ext, "."))
	}

	var results []string
	seen := make(map[string]bool)

	for _, q := range queries {
		if recalled, err := pc.bridge.Recall(q, 500); err == nil && recalled != "" {
			for _, line := range strings.Split(recalled, "\n") {
				key := strings.ToLower(strings.TrimSpace(line))
				if key != "" && !seen[key] && !pc.injectedKeys[key] {
					seen[key] = true
					results = append(results, line)
				}
			}
		}
	}

	if len(results) == 0 {
		return ""
	}

	// Mark as injected so we don't repeat
	for k := range seen {
		pc.injectedKeys[k] = true
	}

	return fmt.Sprintf("## Memories for %s\n%s", basename, strings.Join(results, "\n"))
}

// ContextForTool returns proactive context based on the tool about to be used.
func (pc *ProactiveContext) ContextForTool(toolName string, args map[string]interface{}) string {
	if !pc.bridge.Ready() {
		return ""
	}

	// Extract file path from tool arguments
	if path, ok := extractPath(args); ok && path != "" {
		return pc.ContextForFile(path)
	}

	// For bash commands, try to surface relevant conventions
	if cmd, ok := args["command"].(string); ok && cmd != "" {
		return pc.contextForCommand(cmd)
	}

	return ""
}

func (pc *ProactiveContext) contextForCommand(cmd string) string {
	// Extract the primary command being run
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	// Search for conventions related to this command type
	primary := parts[0]
	recalled, err := pc.bridge.Recall(primary, 300)
	if err != nil || recalled == "" {
		return ""
	}

	// Only return if we haven't already injected this
	key := strings.ToLower(strings.TrimSpace(recalled))
	if pc.injectedKeys[key] {
		return ""
	}
	pc.injectedKeys[key] = true

	return fmt.Sprintf("## Relevant Convention\n%s", recalled)
}

// ContextForActiveFiles returns aggregated context for all currently active files.
// Called at strategic points (e.g., before each LLM query) to keep context fresh.
func (pc *ProactiveContext) ContextForActiveFiles(budget int) string {
	pc.mu.Lock()
	files := make([]string, 0, len(pc.activeFiles))
	for f := range pc.activeFiles {
		files = append(files, f)
	}
	pc.mu.Unlock()

	if len(files) == 0 || !pc.bridge.Ready() {
		return ""
	}

	// Build a combined query from active file basenames
	var queryParts []string
	for _, f := range files {
		queryParts = append(queryParts, filepath.Base(f))
	}
	query := strings.Join(queryParts, " ")

	recalled, err := pc.bridge.Recall(query, budget)
	if err != nil || recalled == "" {
		return ""
	}

	return recalled
}

// ImpactAnalysis checks what memories are affected by a file change.
// Returns formatted impact information or empty string.
func (pc *ProactiveContext) ImpactAnalysis(ctx context.Context, changedFile string) string {
	if !pc.bridge.Ready() || changedFile == "" {
		return ""
	}

	basename := filepath.Base(changedFile)
	recalled, err := pc.bridge.Recall(basename, 500)
	if err != nil || recalled == "" {
		return ""
	}

	lines := strings.Split(recalled, "\n")
	if len(lines) == 0 {
		return ""
	}

	return fmt.Sprintf("⚠ File %s changed — %d memories may be affected:\n%s",
		basename, len(lines), strings.Join(lines, "\n"))
}

// Reset clears injected memory tracking (e.g., at session boundaries).
func (pc *ProactiveContext) Reset() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.injectedKeys = make(map[string]bool)
	pc.activeFiles = make(map[string]time.Time)
}
