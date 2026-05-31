package lsp

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultIdleTimeout = 5 * time.Minute
	defaultInitTimeout = 30 * time.Second
	reaperInterval     = 30 * time.Second
)

// ReadOnlyRetryTools contains tool names that are safe to retry on crash.
var ReadOnlyRetryTools = map[string]bool{
	"diagnostics":     true,
	"goto_definition": true,
	"find_references": true,
	"symbols":         true,
	"prepare_rename":  true,
	"status":          true,
}

// ManagedClient wraps an LSPClient with refcounting and idle tracking.
type ManagedClient struct {
	mu           sync.Mutex
	client       *LSPClient
	config       ServerConfig
	language     string
	refCount     int32
	waiters      int32
	lastUsed     time.Time
	initStart    time.Time
	initializing bool
}

// LSPManager manages a pool of language server connections.
type LSPManager struct {
	mu         sync.RWMutex
	clients    map[string]*ManagedClient // keyed by language
	config     *LSPConfig
	closed     atomic.Bool
	stopReaper context.CancelFunc
}

// NewManager creates an LSPManager with the given config.
func NewManager(cfg *LSPConfig) *LSPManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &LSPManager{
		clients:    make(map[string]*ManagedClient),
		config:     cfg,
		stopReaper: cancel,
	}
	go m.reaper(ctx)
	return m
}

// NewManagerFromProject creates an LSPManager loading config from project directory.
func NewManagerFromProject(projectDir string) *LSPManager {
	return NewManager(LoadConfig(projectDir))
}

func (m *LSPManager) reaper(ctx context.Context) {
	ticker := time.NewTicker(reaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.reapIdle()
		}
	}
}

func (m *LSPManager) reapIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for lang, mc := range m.clients {
		mc.mu.Lock()
		if mc.client != nil && mc.refCount == 0 && mc.waiters == 0 && now.Sub(mc.lastUsed) > defaultIdleTimeout {
			slog.Info("lsp: reaping idle client", "language", lang)
			_ = mc.client.Close()
			mc.client = nil
		}
		// Evict stuck-init clients
		if mc.initializing && now.Sub(mc.initStart) > defaultInitTimeout {
			slog.Warn("lsp: evicting stuck-init client", "language", lang)
			if mc.client != nil {
				_ = mc.client.Close()
				mc.client = nil
			}
			mc.initializing = false
		}
		mc.mu.Unlock()
	}
}

// acquire returns a ready LSPClient for the given language, spawning if needed.
func (m *LSPManager) acquire(ctx context.Context, lang string) (*ManagedClient, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	m.mu.RLock()
	mc, exists := m.clients[lang]
	m.mu.RUnlock()

	if !exists {
		cfg, ok := m.config.Servers[lang]
		if !ok {
			return nil, ErrLanguageNotConfigured
		}
		mc = &ManagedClient{
			config:   cfg,
			language: lang,
		}
		m.mu.Lock()
		// Double-check after acquiring write lock
		if existing, ok := m.clients[lang]; ok {
			m.mu.Unlock()
			mc = existing
		} else {
			m.clients[lang] = mc
			m.mu.Unlock()
		}
	}

	mc.mu.Lock()
	atomic.AddInt32(&mc.waiters, 1)

	// Spawn client if needed
	if mc.client == nil {
		mc.initializing = true
		mc.initStart = time.Now()
		mc.mu.Unlock()

		client, err := NewLSPClient(ctx, lang, mc.config)

		mc.mu.Lock()
		mc.initializing = false
		if err != nil {
			atomic.AddInt32(&mc.waiters, -1)
			mc.mu.Unlock()
			return nil, err
		}
		mc.client = client
	}

	mc.lastUsed = time.Now()
	atomic.AddInt32(&mc.refCount, 1)
	atomic.AddInt32(&mc.waiters, -1)
	mc.mu.Unlock()

	return mc, nil
}

// release decrements the refcount on a managed client.
func (mc *ManagedClient) release() {
	atomic.AddInt32(&mc.refCount, -1)
	mc.mu.Lock()
	mc.lastUsed = time.Now()
	mc.mu.Unlock()
}

// Execute runs fn with an LSPClient, handling acquire/release and crash recovery.
// readOnly indicates if the operation is idempotent (safe to retry once on crash).
func (m *LSPManager) Execute(ctx context.Context, lang string, readOnly bool, fn func(client *LSPClient) error) error {
	return m.executeWithRetry(ctx, lang, readOnly, fn, true)
}

func (m *LSPManager) executeWithRetry(ctx context.Context, lang string, readOnly bool, fn func(client *LSPClient) error, allowRetry bool) error {
	mc, err := m.acquire(ctx, lang)
	if err != nil {
		return err
	}
	defer mc.release()

	err = fn(mc.client)
	if err != nil && readOnly && allowRetry {
		// Crash recovery: close and retry once
		slog.Warn("lsp: retrying after error", "language", lang, "error", err)
		mc.mu.Lock()
		if mc.client != nil {
			_ = mc.client.Close()
			mc.client = nil
		}
		mc.mu.Unlock()

		return m.executeWithRetry(ctx, lang, readOnly, fn, false)
	}
	return err
}

// Status returns the state of all configured language servers.
func (m *LSPManager) Status() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := make(map[string]string)
	for lang := range m.config.Servers {
		if mc, ok := m.clients[lang]; ok {
			mc.mu.Lock()
			if mc.client != nil {
				status[lang] = "connected"
			} else if mc.initializing {
				status[lang] = "initializing"
			} else {
				status[lang] = "idle"
			}
			mc.mu.Unlock()
		} else {
			status[lang] = "available"
		}
	}
	return status
}

// Config returns the LSP configuration.
func (m *LSPManager) Config() *LSPConfig {
	return m.config
}

// Close shuts down all language server connections.
func (m *LSPManager) Close() error {
	if m.closed.Swap(true) {
		return nil
	}
	m.stopReaper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mc := range m.clients {
		mc.mu.Lock()
		if mc.client != nil {
			_ = mc.client.Close()
			mc.client = nil
		}
		mc.mu.Unlock()
	}
	return nil
}

// Errors
var (
	ErrManagerClosed         = &LSPError{Code: "MANAGER_CLOSED", Message: "LSP manager is closed"}
	ErrLanguageNotConfigured = &LSPError{Code: "LANG_NOT_CONFIGURED", Message: "no LSP server configured for language"}
)

type LSPError struct {
	Code    string
	Message string
}

func (e *LSPError) Error() string { return e.Code + ": " + e.Message }
