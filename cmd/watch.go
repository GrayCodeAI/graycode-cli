package cmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches a directory tree for file changes, debounces them,
// computes a git diff, and calls an onChange callback.
type FileWatcher struct {
	dir      string
	ignore   []string
	onChange func(path, diff string)
	done     chan struct{}
	once     sync.Once
}

// NewFileWatcher creates a new FileWatcher.  ignore is a list of path
// substrings to skip (e.g. ".git", "node_modules").  onChange is called
// with the changed file path and the git diff for that file.
func NewFileWatcher(dir string, ignore []string, onChange func(string, string)) *FileWatcher {
	return &FileWatcher{
		dir:      dir,
		ignore:   ignore,
		onChange: onChange,
		done:     make(chan struct{}),
	}
}

// Start begins watching for file changes.  It blocks until ctx is
// cancelled or Stop is called.
func (fw *FileWatcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(fw.dir); err != nil {
		return err
	}

	const debounce = 300 * time.Millisecond
	var (
		mu      sync.Mutex
		pending = make(map[string]time.Time)
	)

	// Flush goroutine: fires debounced callbacks.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-fw.done:
				return
			case <-ticker.C:
				mu.Lock()
				now := time.Now()
				for p, t := range pending {
					if now.Sub(t) >= debounce {
						delete(pending, p)
						go fw.fireChange(p)
					}
				}
				mu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fw.done:
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if fw.shouldIgnore(event.Name) {
				continue
			}
			mu.Lock()
			pending[event.Name] = time.Now()
			mu.Unlock()
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			_ = err // log if needed
		}
	}
}

// Stop signals the watcher to shut down.
func (fw *FileWatcher) Stop() {
	fw.once.Do(func() {
		close(fw.done)
	})
}

// shouldIgnore returns true if the path matches any ignore pattern.
func (fw *FileWatcher) shouldIgnore(path string) bool {
	for _, pattern := range fw.ignore {
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

// fireChange computes the git diff for the given file and calls onChange.
func (fw *FileWatcher) fireChange(path string) {
	diff := gitDiffForFile(fw.dir, path)
	if fw.onChange != nil {
		fw.onChange(path, diff)
	}
}

// gitDiffForFile runs git diff for a specific file relative to dir.
func gitDiffForFile(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		rel = path
	}
	cmd := exec.Command("git", "diff", "--", rel)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
