package repomap

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher monitors the project directory for changes and triggers re-indexing.
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	root     string
	onChange func(path string)
	done     chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewFileWatcher creates a watcher for Go/Python/TS files in the given root.
func NewFileWatcher(root string, onChange func(path string)) (*FileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:  w,
		root:     root,
		onChange: onChange,
		done:     make(chan struct{}),
	}

	// Add all directories
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			_ = w.Add(path)
		}
		return nil
	})

	return fw, nil
}

// Start begins watching for file changes in a goroutine.
func (fw *FileWatcher) Start() {
	fw.mu.Lock()
	if fw.running {
		fw.mu.Unlock()
		return
	}
	fw.running = true
	fw.mu.Unlock()

	go fw.loop()
}

// Stop terminates the watcher.
func (fw *FileWatcher) Stop() {
	fw.mu.Lock()
	if !fw.running {
		fw.mu.Unlock()
		return
	}
	fw.running = false
	fw.mu.Unlock()
	close(fw.done)
	_ = fw.watcher.Close()
}

func (fw *FileWatcher) loop() {
	for {
		select {
		case <-fw.done:
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) == 0 {
				continue
			}
			if !isSourceFile(event.Name) {
				continue
			}
			if fw.onChange != nil {
				fw.onChange(event.Name)
			}
		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func isSourceFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java", ".rb":
		return true
	}
	return false
}
