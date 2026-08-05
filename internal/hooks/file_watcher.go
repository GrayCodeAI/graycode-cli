package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var (
	watcherMu sync.Mutex
	watcher   *fsnotify.Watcher
	watchDirs = make(map[string]bool)
)

// InitFileWatcher starts the global file watcher and bridges fsnotify events
// to the hooks EventFileChanged event.
func InitFileWatcher(ctx context.Context) error {
	watcherMu.Lock()
	defer watcherMu.Unlock()

	if watcher != nil {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	watcher = w
	go runFileWatcher(ctx)
	return nil
}

// WatchDir adds a directory to the file watch list.
func WatchDir(dir string) error {
	watcherMu.Lock()
	defer watcherMu.Unlock()

	if watcher == nil {
		return nil
	}
	if watchDirs[dir] {
		return nil
	}
	if err := watcher.Add(dir); err != nil {
		return err
	}
	watchDirs[dir] = true
	return nil
}

// CloseFileWatcher stops the global file watcher.
func CloseFileWatcher() error {
	watcherMu.Lock()
	defer watcherMu.Unlock()

	if watcher == nil {
		return nil
	}
	err := watcher.Close()
	watcher = nil
	watchDirs = make(map[string]bool)
	return err
}

func runFileWatcher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				rel := relPath(event.Name)
				if shouldIgnore(rel) {
					continue
				}
				ExecuteAsync(ctx, EventFileChanged, map[string]interface{}{
					"path": rel,
					"op":   event.Op.String(),
					"file": filepath.Base(event.Name),
					"dir":  filepath.Dir(rel),
					"abs":  event.Name,
				})
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func relPath(abs string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return abs
	}
	return rel
}

func shouldIgnore(path string) bool {
	ignored := []string{".git", ".hawk/specs", "node_modules", ".DS_Store", "vendor"}
	for _, p := range ignored {
		if strings.Contains(path, string(os.PathSeparator)+p+string(os.PathSeparator)) ||
			strings.HasPrefix(path, p+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func init() {
	_ = isTestFile
	_ = isSourceFile
}

func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".test.ts") ||
		strings.HasSuffix(path, ".test.js") || strings.HasSuffix(path, "_test.py")
}

func isSourceFile(path string) bool {
	return strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".ts") ||
		strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".py") ||
		strings.HasSuffix(path, ".rs") || strings.HasSuffix(path, ".java") ||
		strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".cpp")
}
