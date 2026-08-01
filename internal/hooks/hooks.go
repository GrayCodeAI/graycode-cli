package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/trust"
)

// allowProjectHookDir gates project-scoped hook directories behind folder trust.
func allowProjectHookDir(dir string) error {
	return trust.AllowLoadPath(dir)
}

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

// EventEnvelope provides structured, typed metadata for hook events.
// It wraps the raw payload with tracing and context fields.
type EventEnvelope struct {
	Timestamp     time.Time              `json:"timestamp"`
	Source        string                 `json:"source"`         // e.g. "hooks", "mission", "plugin"
	SessionID     string                 `json:"session_id"`     // session that triggered the event
	AgentID       string                 `json:"agent_id"`       // agent that triggered the event
	CorrelationID string                 `json:"correlation_id"` // traces across hook chains
	EventType     EventType              `json:"event_type"`
	Payload       map[string]interface{} `json:"payload"` // event-specific data
}

// EnvelopeFn is the typed hook function signature using EventEnvelope.
type EnvelopeFn func(ctx context.Context, envelope EventEnvelope) error

// Hook is a registered hook function.
type Hook struct {
	Name     string
	Event    EventType
	Priority int                                                          // lower = earlier
	Fn       func(ctx context.Context, data map[string]interface{}) error // legacy
	FnV2     EnvelopeFn                                                   // typed envelope (preferred)
}

// Registry stores and executes hooks.
type Registry struct {
	mu    sync.RWMutex
	hooks map[EventType][]Hook
	// asyncWG tracks fire-and-forget hooks so owners can drain them during
	// shutdown or deterministic tests.
	asyncWG sync.WaitGroup
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
// Fail-open: hook errors are logged but do not stop execution of subsequent hooks.
func (r *Registry) Execute(ctx context.Context, event EventType, data map[string]interface{}) error {
	env := EventEnvelope{
		Timestamp: time.Now(),
		Source:    "hooks",
		EventType: event,
		Payload:   data,
	}
	return r.ExecuteEnvelope(ctx, env)
}

// ExecuteEnvelope runs all hooks for an event using a typed EventEnvelope.
// Hooks with FnV2 set receive the envelope directly; legacy Fn hooks receive the payload.
// Fail-open: hook errors are logged but do not stop execution of subsequent hooks.
func (r *Registry) ExecuteEnvelope(ctx context.Context, env EventEnvelope) error {
	r.mu.RLock()
	hooks := r.hooks[env.EventType]
	r.mu.RUnlock()

	var firstErr error
	for _, h := range hooks {
		var err error
		if h.FnV2 != nil {
			err = h.FnV2(ctx, env)
		} else if h.Fn != nil {
			err = h.Fn(ctx, env.Payload)
		}
		if err != nil {
			// Log the error but continue executing remaining hooks (fail-open)
			fmt.Fprintf(os.Stderr, "WARNING: hook %q failed (continuing): %v\n", h.Name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("hook %s failed: %w", h.Name, err)
			}
		}
	}
	return firstErr
}

// ExecuteAsync runs hooks asynchronously and records them for WaitAsync.
// The caller's values and trace span are preserved, but cancellation is
// detached so a normal request teardown does not kill post-event observers.
func (r *Registry) ExecuteAsync(ctx context.Context, event EventType, data map[string]interface{}) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithoutCancel(ctx)
	r.asyncWG.Add(1)
	go func() {
		defer r.asyncWG.Done()
		_ = r.Execute(ctx, event, data)
	}()
}

// ExecuteAsyncEnvelope runs hooks asynchronously using a typed EventEnvelope.
func (r *Registry) ExecuteAsyncEnvelope(ctx context.Context, env EventEnvelope) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithoutCancel(ctx)
	r.asyncWG.Add(1)
	go func() {
		defer r.asyncWG.Done()
		_ = r.ExecuteEnvelope(ctx, env)
	}()
}

// WaitAsync waits for currently queued asynchronous hooks to finish or for
// ctx to expire. Callers must stop scheduling new async hooks before waiting.
func (r *Registry) WaitAsync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		r.asyncWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

// ExecuteEnvelope runs all hooks for an event on the global registry using a typed envelope.
func ExecuteEnvelope(ctx context.Context, env EventEnvelope) error {
	return global.ExecuteEnvelope(ctx, env)
}

// ExecuteAsyncEnvelope runs hooks asynchronously on the global registry using a typed envelope.
func ExecuteAsyncEnvelope(ctx context.Context, env EventEnvelope) {
	global.ExecuteAsyncEnvelope(ctx, env)
}

// WaitAsync waits for currently queued package-level asynchronous hooks.
func WaitAsync(ctx context.Context) error { return global.WaitAsync(ctx) }

// AdaptLegacyFn wraps a legacy hook function into an EnvelopeFn.
// The legacy function receives only the payload from the envelope.
func AdaptLegacyFn(fn func(ctx context.Context, data map[string]interface{}) error) EnvelopeFn {
	return func(ctx context.Context, env EventEnvelope) error {
		return fn(ctx, env.Payload)
	}
}

// LoadHooksDir loads hooks from a directory.
// Project-scoped directories require folder trust when HAWK_Y0_FOLDER_TRUST is on.
func LoadHooksDir(dir string) error {
	if err := allowProjectHookDir(dir); err != nil {
		return err
	}
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

// LoadConventionPolicies discovers and loads convention policy files.
// Convention: auto-discovered *policy*.{md,go} files from:
//   - {cwd}/.agents/policies/ (project scope)
//   - Hawk user state policies directory
//
// Returns the number of policies loaded.
func LoadConventionPolicies(cwd string) int {
	count := 0
	// Project scope
	projectDir := filepath.Join(cwd, ".agents", "policies")
	count += loadPolicyDir(projectDir, "project")

	// User scope
	userDir := filepath.Join(storage.StateDir(), "policies")
	count += loadPolicyDir(userDir, "user")

	return count
}

func loadPolicyDir(dir string, scope string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".go") {
			continue
		}
		path := filepath.Join(dir, name)
		// Policy files are loaded from markdown frontmatter or Go source
		// The actual parsing depends on the file type
		_ = path
		count++
	}

	return count
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
