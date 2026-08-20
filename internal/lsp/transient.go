package lsp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// MaxSourceFileSizeBytes limits source reading for transient open to 10 MiB.
	MaxSourceFileSizeBytes = 10 * 1024 * 1024
)

// WorkspaceQueue manages per-workspace FIFO execution queues.
type WorkspaceQueue struct {
	mu     sync.Mutex
	queues map[string]chan struct{}
}

var defaultWorkspaceQueue = NewWorkspaceQueue()

// NewWorkspaceQueue creates an isolated WorkspaceQueue.
func NewWorkspaceQueue() *WorkspaceQueue {
	return &WorkspaceQueue{
		queues: make(map[string]chan struct{}),
	}
}

// GetWorkspaceQueue returns the global singleton WorkspaceQueue.
func GetWorkspaceQueue() *WorkspaceQueue {
	return defaultWorkspaceQueue
}

// Lock acquires the execution token for the workspace. Call the returned unlock func when done.
func (wq *WorkspaceQueue) Lock(ctx context.Context, workspaceDir string) (func(), error) {
	cleanDir := filepath.Clean(workspaceDir)

	wq.mu.Lock()
	q, exists := wq.queues[cleanDir]
	if !exists {
		q = make(chan struct{}, 1)
		q <- struct{}{}
		wq.queues[cleanDir] = q
	}
	wq.mu.Unlock()

	select {
	case <-q:
		unlock := func() {
			q <- struct{}{}
		}
		return unlock, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReadBoundedSource reads up to MaxSourceFileSizeBytes from a file path.
func ReadBoundedSource(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path provided by developer/agent
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	reader := io.LimitReader(f, MaxSourceFileSizeBytes+1)
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if len(bytes) > MaxSourceFileSizeBytes {
		return "", fmt.Errorf("lsp: source file %s exceeds %d byte limit", path, MaxSourceFileSizeBytes)
	}
	return string(bytes), nil
}

// ExecuteTransient performs a transient-open query lifecycle:
// 1. Serializes through the workspace queue.
// 2. Reads the current bounded file text from disk.
// 3. Sends textDocument/didOpen (version 1, full text).
// 4. Runs the requested queryFn.
// 5. In finally (defer), sends textDocument/didClose.
// If didOpen fails or context is canceled, the server connection is closed/evicted.
func (m *LSPManager) ExecuteTransient(
	ctx context.Context,
	workspaceDir string,
	filePath string,
	lang string,
	readOnly bool,
	queryFn func(client *LSPClient, uri string) error,
) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	wq := m.workspaceQueue
	if wq == nil {
		wq = defaultWorkspaceQueue
	}

	unlock, err := wq.Lock(ctx, workspaceDir)
	if err != nil {
		return fmt.Errorf("lsp: workspace queue: %w", err)
	}
	defer unlock()

	// Read fresh source on turn start
	content, err := ReadBoundedSource(filePath)
	if err != nil {
		return fmt.Errorf("lsp: read source: %w", err)
	}

	uri := FileURI(filePath)

	return m.Execute(ctx, lang, readOnly, func(client *LSPClient) error {
		// didOpen version 1
		openErr := client.DidOpen(ctx, uri, lang, 1, content)
		if openErr != nil {
			// Failed didOpen terminates the server instance before pool reuse
			_ = client.Close()
			return fmt.Errorf("lsp: transient didOpen failed: %w", openErr)
		}

		defer func() {
			_ = client.DidClose(context.Background(), uri)
		}()

		return queryFn(client, uri)
	})
}

// FileURI formats a file path into an LSP file:// URI.
func FileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}
