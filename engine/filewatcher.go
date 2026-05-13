package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileEvent represents a single file system change event.
type FileEvent struct {
	Path string
	Type string // "create", "modify", "delete", "rename"
	Time time.Time
	Size int64
}

// WatcherConfig holds configuration for a FileWatcher.
type WatcherConfig struct {
	Patterns       []string
	IgnorePatterns []string
	Debounce       time.Duration
	BatchWindow    time.Duration
	PollInterval   time.Duration
}

// FileWatcher monitors a directory tree for file changes using polling.
// It supports glob-based inclusion/exclusion patterns, debouncing of rapid
// changes, and batching of events within a configurable window.
type FileWatcher struct {
	RootDir        string
	Patterns       []string
	IgnorePatterns []string
	Debounce       time.Duration
	BatchWindow    time.Duration
	OnChange       func([]FileEvent)

	polling  bool
	interval time.Duration
	done     chan struct{}
	mu       sync.Mutex
}

// fileSnapshot stores mod time and size for change detection.
type fileSnapshot struct {
	modTime time.Time
	size    int64
}

// NewFileWatcher creates a FileWatcher with the given root directory and config.
// Defaults: Debounce 500ms, BatchWindow 100ms, PollInterval 1s.
func NewFileWatcher(rootDir string, config WatcherConfig) *FileWatcher {
	debounce := config.Debounce
	if debounce == 0 {
		debounce = 500 * time.Millisecond
	}
	batchWindow := config.BatchWindow
	if batchWindow == 0 {
		batchWindow = 100 * time.Millisecond
	}
	pollInterval := config.PollInterval
	if pollInterval == 0 {
		pollInterval = 1 * time.Second
	}
	ignorePatterns := config.IgnorePatterns
	if len(ignorePatterns) == 0 {
		ignorePatterns = DefaultIgnorePatterns()
	}

	return &FileWatcher{
		RootDir:        rootDir,
		Patterns:       config.Patterns,
		IgnorePatterns: ignorePatterns,
		Debounce:       debounce,
		BatchWindow:    batchWindow,
		OnChange:       nil,
		polling:        false,
		interval:       pollInterval,
		done:           make(chan struct{}),
	}
}

// Start begins the polling-based file watcher. It walks the directory at each
// PollInterval, detects creates/modifies/deletes, batches events within
// BatchWindow, and debounces rapid changes (only fires OnChange after Debounce
// duration of quiet). It blocks until ctx is cancelled or Stop is called.
func (fw *FileWatcher) Start(ctx context.Context) error {
	fw.mu.Lock()
	if fw.polling {
		fw.mu.Unlock()
		return fmt.Errorf("watcher already started")
	}
	fw.polling = true
	fw.done = make(chan struct{})
	fw.mu.Unlock()

	// Initial scan to establish baseline.
	prev := fw.scan()

	var pendingEvents []FileEvent
	var debounceTimer *time.Timer
	var debounceC <-chan time.Time

	ticker := time.NewTicker(fw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fw.mu.Lock()
			fw.polling = false
			fw.mu.Unlock()
			// Fire any pending events before exit.
			if len(pendingEvents) > 0 && fw.OnChange != nil {
				deduped := DedupEvents(pendingEvents)
				fw.OnChange(deduped)
			}
			return ctx.Err()

		case <-fw.done:
			fw.mu.Lock()
			fw.polling = false
			fw.mu.Unlock()
			// Fire any pending events before exit.
			if len(pendingEvents) > 0 && fw.OnChange != nil {
				deduped := DedupEvents(pendingEvents)
				fw.OnChange(deduped)
			}
			return nil

		case <-ticker.C:
			current := fw.scan()
			events := fw.diff(prev, current)
			if len(events) > 0 {
				pendingEvents = append(pendingEvents, events...)
				// Reset debounce timer on each new batch of changes.
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(fw.Debounce)
				debounceC = debounceTimer.C
			}
			prev = current

		case <-debounceC:
			// Debounce period elapsed with no new changes — fire events.
			if len(pendingEvents) > 0 && fw.OnChange != nil {
				deduped := DedupEvents(pendingEvents)
				fw.OnChange(deduped)
			}
			pendingEvents = nil
			debounceTimer = nil
			debounceC = nil
		}
	}
}

// Stop signals the watcher to stop polling.
func (fw *FileWatcher) Stop() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.polling {
		close(fw.done)
	}
}

// scan walks the root directory and builds a snapshot of all tracked files.
func (fw *FileWatcher) scan() map[string]fileSnapshot {
	result := make(map[string]fileSnapshot)
	_ = filepath.Walk(fw.RootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Get relative path for pattern matching.
		rel, relErr := filepath.Rel(fw.RootDir, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}

		// Check if directory should be ignored (skip entire subtree).
		if info.IsDir() {
			if fw.ShouldIgnore(rel + "/") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file should be ignored.
		if fw.ShouldIgnore(rel) {
			return nil
		}

		// If patterns are specified, file must match at least one.
		if len(fw.Patterns) > 0 && !MatchesPattern(rel, fw.Patterns) {
			return nil
		}

		result[rel] = fileSnapshot{
			modTime: info.ModTime(),
			size:    info.Size(),
		}
		return nil
	})
	return result
}

// diff compares two snapshots and produces file events.
func (fw *FileWatcher) diff(prev, current map[string]fileSnapshot) []FileEvent {
	var events []FileEvent
	now := time.Now()

	// Detect creates and modifies.
	for path, cur := range current {
		old, exists := prev[path]
		if !exists {
			events = append(events, FileEvent{
				Path: path,
				Type: "create",
				Time: now,
				Size: cur.size,
			})
		} else if cur.modTime != old.modTime || cur.size != old.size {
			events = append(events, FileEvent{
				Path: path,
				Type: "modify",
				Time: now,
				Size: cur.size,
			})
		}
	}

	// Detect deletes.
	for path := range prev {
		if _, exists := current[path]; !exists {
			events = append(events, FileEvent{
				Path: path,
				Type: "delete",
				Time: now,
				Size: 0,
			})
		}
	}

	return events
}

// DefaultIgnorePatterns returns standard patterns to ignore during watching.
func DefaultIgnorePatterns() []string {
	return []string{
		".git/",
		"node_modules/",
		"vendor/",
		"__pycache__/",
		".venv/",
		"dist/",
		"build/",
		".DS_Store",
		"*.swp",
		"*.swo",
		"*~",
	}
}

// MatchesPattern checks if a path matches any of the given glob patterns.
func MatchesPattern(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Try matching against full path.
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Try matching against just the filename.
		base := filepath.Base(path)
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Try matching each path component for directory patterns.
		parts := strings.Split(path, string(filepath.Separator))
		for _, part := range parts {
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
		}
	}
	return false
}

// ShouldIgnore checks whether a path matches any of the watcher's ignore patterns.
func (fw *FileWatcher) ShouldIgnore(path string) bool {
	return matchesIgnorePattern(path, fw.IgnorePatterns)
}

// matchesIgnorePattern checks if path matches any ignore pattern.
func matchesIgnorePattern(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Directory pattern (ending with /).
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			// Check if any path component matches.
			parts := strings.Split(path, string(filepath.Separator))
			for _, part := range parts {
				// Strip trailing slash from part if present.
				cleanPart := strings.TrimSuffix(part, "/")
				if matched, _ := filepath.Match(dirPattern, cleanPart); matched {
					return true
				}
			}
			continue
		}

		// File pattern — match against full path or basename.
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		base := filepath.Base(strings.TrimSuffix(path, "/"))
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Match against each component.
		parts := strings.Split(path, string(filepath.Separator))
		for _, part := range parts {
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
		}
	}
	return false
}

// DedupEvents removes duplicate events for the same path within a batch,
// keeping only the latest event for each path.
func DedupEvents(events []FileEvent) []FileEvent {
	seen := make(map[string]int) // path -> index in result
	var result []FileEvent

	for _, ev := range events {
		if idx, exists := seen[ev.Path]; exists {
			// Replace with the later event.
			result[idx] = ev
		} else {
			seen[ev.Path] = len(result)
			result = append(result, ev)
		}
	}
	return result
}

// FormatEvents produces a human-readable summary of file events.
func FormatEvents(events []FileEvent) string {
	if len(events) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("File changes detected:\n")
	for _, ev := range events {
		var prefix string
		var detail string
		switch ev.Type {
		case "create":
			prefix = "A"
			detail = "(new)"
		case "modify":
			prefix = "M"
			detail = fmt.Sprintf("(%s)", formatSize(ev.Size))
		case "delete":
			prefix = "D"
			detail = "(deleted)"
		case "rename":
			prefix = "R"
			detail = "(renamed)"
		default:
			prefix = "?"
			detail = ""
		}
		sb.WriteString(fmt.Sprintf("  %s %s %s\n", prefix, ev.Path, detail))
	}
	return sb.String()
}

// formatSize returns a human-readable file size string.
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(size)/1024.0)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(size)/(1024.0*1024.0))
	}
	return fmt.Sprintf("%.1fGB", float64(size)/(1024.0*1024.0*1024.0))
}

// SingleFileWatcher provides a simpler API for watching a single file.
type SingleFileWatcher struct {
	path     string
	onChange func()
	done     chan struct{}
	mu       sync.Mutex
	running  bool
	interval time.Duration
}

// WatchSingle creates a SingleFileWatcher for monitoring a single file.
func WatchSingle(path string, onChange func()) *SingleFileWatcher {
	return &SingleFileWatcher{
		path:     path,
		onChange: onChange,
		done:     make(chan struct{}),
		interval: 500 * time.Millisecond,
	}
}

// Start begins polling the single file for changes. Blocks until ctx is
// cancelled or Stop is called.
func (sw *SingleFileWatcher) Start(ctx context.Context) error {
	sw.mu.Lock()
	if sw.running {
		sw.mu.Unlock()
		return fmt.Errorf("single file watcher already started")
	}
	sw.running = true
	sw.done = make(chan struct{})
	sw.mu.Unlock()

	info, _ := os.Stat(sw.path)
	var lastMod time.Time
	var lastSize int64
	if info != nil {
		lastMod = info.ModTime()
		lastSize = info.Size()
	}

	ticker := time.NewTicker(sw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sw.mu.Lock()
			sw.running = false
			sw.mu.Unlock()
			return ctx.Err()
		case <-sw.done:
			sw.mu.Lock()
			sw.running = false
			sw.mu.Unlock()
			return nil
		case <-ticker.C:
			current, err := os.Stat(sw.path)
			if err != nil {
				// File might have been deleted then recreated.
				if info != nil {
					// Was present, now gone — trigger change.
					info = nil
					lastMod = time.Time{}
					lastSize = 0
					if sw.onChange != nil {
						sw.onChange()
					}
				}
				continue
			}
			if current.ModTime() != lastMod || current.Size() != lastSize {
				lastMod = current.ModTime()
				lastSize = current.Size()
				info = current
				if sw.onChange != nil {
					sw.onChange()
				}
			}
		}
	}
}

// Stop signals the single file watcher to stop.
func (sw *SingleFileWatcher) Stop() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.running {
		close(sw.done)
	}
}
