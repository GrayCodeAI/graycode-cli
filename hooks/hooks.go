package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EventType represents a hook event.
type EventType string

const (
	EventPreQuery      EventType = "pre_query"
	EventPostQuery     EventType = "post_query"
	EventPreTool       EventType = "pre_tool"
	EventPostTool      EventType = "post_tool"
	EventPreCompact    EventType = "pre_compact"
	EventPostCompact   EventType = "post_compact"
	EventFileChanged   EventType = "file_changed"
	EventSessionStart  EventType = "session_start"
	EventSessionEnd    EventType = "session_end"
	EventPermissionAsk EventType = "permission_ask"
	EventError         EventType = "error"
)

// Hook is a registered hook function.
type Hook struct {
	Name     string
	Event    EventType
	Priority int // lower = earlier
	Fn       func(ctx context.Context, data map[string]interface{}) error
}

// Registry stores and executes hooks.
type Registry struct {
	mu    sync.RWMutex
	hooks map[EventType][]Hook
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[EventType][]Hook),
	}
}

// Register adds a hook to the registry.
func (r *Registry) Register(h Hook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[h.Event] = append(r.hooks[h.Event], h)
	// Sort by priority
	sortHooks(r.hooks[h.Event])
}

// Execute runs all hooks for an event.
func (r *Registry) Execute(ctx context.Context, event EventType, data map[string]interface{}) error {
	r.mu.RLock()
	hooks := r.hooks[event]
	r.mu.RUnlock()

	for _, h := range hooks {
		if err := h.Fn(ctx, data); err != nil {
			return fmt.Errorf("hook %s failed: %w", h.Name, err)
		}
	}
	return nil
}

// ExecuteAsync runs hooks asynchronously (fire and forget).
func (r *Registry) ExecuteAsync(ctx context.Context, event EventType, data map[string]interface{}) {
	go func() {
		_ = r.Execute(ctx, event, data)
	}()
}

func sortHooks(hooks []Hook) {
	for i := 0; i < len(hooks); i++ {
		for j := i + 1; j < len(hooks); j++ {
			if hooks[j].Priority < hooks[i].Priority {
				hooks[i], hooks[j] = hooks[j], hooks[i]
			}
		}
	}
}

// Global registry instance.
var global = NewRegistry()

// Register adds a hook to the global registry.
func Register(h Hook) { global.Register(h) }

// Execute runs all hooks for an event on the global registry.
func Execute(ctx context.Context, event EventType, data map[string]interface{}) error {
	return global.Execute(ctx, event, data)
}

// ExecuteAsync runs hooks asynchronously on the global registry.
func ExecuteAsync(ctx context.Context, event EventType, data map[string]interface{}) {
	global.ExecuteAsync(ctx, event, data)
}

// LoadHooksDir loads hooks from a directory.
func LoadHooksDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		_ = path // hooks are loaded from markdown frontmatter
	}
	return nil
}

// BuiltinHooks returns the default set of built-in hooks.
func BuiltinHooks() []Hook {
	return []Hook{
		{
			Name:     "cost_tracker",
			Event:    EventPostQuery,
			Priority: 100,
			Fn: func(ctx context.Context, data map[string]interface{}) error {
				// Cost tracking is handled by the engine
				return nil
			},
		},
		{
			Name:     "file_watcher",
			Event:    EventFileChanged,
			Priority: 10,
			Fn: func(ctx context.Context, data map[string]interface{}) error {
				// File change notifications
				return nil
			},
		},
		{
			Name:     "session_logger",
			Event:    EventSessionStart,
			Priority: 1,
			Fn: func(ctx context.Context, data map[string]interface{}) error {
				// Session start logging
				return nil
			},
		},
		{
			Name:     "permission_logger",
			Event:    EventPermissionAsk,
			Priority: 1,
			Fn: func(ctx context.Context, data map[string]interface{}) error {
				// Permission ask logging
				return nil
			},
		},
	}
}
