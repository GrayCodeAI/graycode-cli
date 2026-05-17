package hooks

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// LifecycleEventType constants representing agent lifecycle events.
const (
	SessionStart    = "session.start"
	SessionEnd      = "session.end"
	TurnStart       = "turn.start"
	TurnEnd         = "turn.end"
	ToolCallStart   = "tool_call.start"
	ToolCallEnd     = "tool_call.end"
	ToolCallError   = "tool_call.error"
	FileRead        = "file.read"
	FileWrite       = "file.write"
	FileEdit        = "file.edit"
	FileDelete      = "file.delete"
	CompactionStart = "compaction.start"
	CompactionEnd   = "compaction.end"
	BudgetWarning   = "budget.warning"
	BudgetExceeded  = "budget.exceeded"
	ErrorOccurred   = "error.occurred"
	ErrorRecovered  = "error.recovered"
	ModelSwitch     = "model.switch"
	ProviderSwitch  = "provider.switch"
	UserInput       = "user.input"
	AgentResponse   = "agent.response"

	// Review lifecycle events
	ReviewQueued    = "review.queued"
	ReviewStarted   = "review.started"
	ReviewCompleted = "review.completed"
	ReviewFailed    = "review.failed"
	ReviewFixed     = "review.fixed"
)

// Event represents a single lifecycle event emitted by the agent.
type Event struct {
	Name      string
	Timestamp time.Time
	Data      map[string]interface{}
	Source    string
}

// LifecycleHook is a registered handler for lifecycle events on the EventBus.
type LifecycleHook struct {
	ID       string
	Name     string
	Event    string
	Handler  func(Event) error
	Priority int
	Async    bool
	Enabled  bool
}

// EventStats provides aggregate statistics about the event bus.
type EventStats struct {
	TotalEvents int
	ByType      map[string]int
	HookCount   int
	AsyncHooks  int
	AvgHookTime time.Duration
}

// EventBus is the central publish/subscribe mechanism for lifecycle events.
type EventBus struct {
	Hooks      map[string][]*LifecycleHook
	Listeners  map[string][]chan Event
	History    []Event
	MaxHistory int

	mu            sync.RWMutex
	hookTimeTotal time.Duration
	hookCallCount int64
}

// NewEventBus creates a new EventBus with sensible defaults.
func NewEventBus() *EventBus {
	return &EventBus{
		Hooks:      make(map[string][]*LifecycleHook),
		Listeners:  make(map[string][]chan Event),
		History:    make([]Event, 0, 256),
		MaxHistory: 1000,
	}
}

// Register adds a hook to the bus for its configured event type.
func (eb *EventBus) Register(hook *LifecycleHook) {
	if hook == nil {
		return
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.Hooks[hook.Event] = append(eb.Hooks[hook.Event], hook)
	sort.SliceStable(eb.Hooks[hook.Event], func(i, j int) bool {
		return eb.Hooks[hook.Event][i].Priority < eb.Hooks[hook.Event][j].Priority
	})
}

// Unregister removes a hook by its ID from all event types.
func (eb *EventBus) Unregister(hookID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for eventType, hooks := range eb.Hooks {
		filtered := make([]*LifecycleHook, 0, len(hooks))
		for _, h := range hooks {
			if h.ID != hookID {
				filtered = append(filtered, h)
			}
		}
		eb.Hooks[eventType] = filtered
	}
}

// Emit fires all hooks for the event type and sends to listeners.
// Synchronous hooks run in priority order; async hooks run in goroutines.
func (eb *EventBus) Emit(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	eb.mu.Lock()
	if len(eb.History) >= eb.MaxHistory {
		// Drop oldest 10% to amortize trimming cost.
		drop := eb.MaxHistory / 10
		if drop < 1 {
			drop = 1
		}
		eb.History = eb.History[drop:]
	}
	eb.History = append(eb.History, event)
	eb.mu.Unlock()

	eb.mu.RLock()
	hooks := make([]*LifecycleHook, len(eb.Hooks[event.Name]))
	copy(hooks, eb.Hooks[event.Name])
	listeners := make([]chan Event, len(eb.Listeners[event.Name]))
	copy(listeners, eb.Listeners[event.Name])
	eb.mu.RUnlock()

	// Execute synchronous hooks in priority order.
	for _, h := range hooks {
		if !h.Enabled {
			continue
		}
		if h.Async {
			hCopy := h
			go func() {
				start := time.Now()
				_ = hCopy.Handler(event)
				eb.recordHookTime(time.Since(start))
			}()
		} else {
			start := time.Now()
			_ = h.Handler(event)
			eb.recordHookTime(time.Since(start))
		}
	}

	// Send to channel-based listeners (non-blocking).
	for _, ch := range listeners {
		select {
		case ch <- event:
		default:
			// Drop if listener is not keeping up.
		}
	}
}

func (eb *EventBus) recordHookTime(d time.Duration) {
	eb.mu.Lock()
	eb.hookTimeTotal += d
	eb.hookCallCount++
	eb.mu.Unlock()
}

// Subscribe returns a channel that receives events of the given type.
func (eb *EventBus) Subscribe(eventType string) <-chan Event {
	ch := make(chan Event, 64)
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.Listeners[eventType] = append(eb.Listeners[eventType], ch)
	return ch
}

// Unsubscribe removes a previously subscribed channel.
func (eb *EventBus) Unsubscribe(eventType string, ch <-chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	listeners := eb.Listeners[eventType]
	filtered := make([]chan Event, 0, len(listeners))
	for _, l := range listeners {
		if l != ch {
			filtered = append(filtered, l)
		}
	}
	eb.Listeners[eventType] = filtered
}

// OnFileWrite registers a convenience hook that fires on FileWrite events.
func (eb *EventBus) OnFileWrite(fn func(path string)) {
	eb.Register(&LifecycleHook{
		ID:      fmt.Sprintf("on_file_write_%p", fn),
		Name:    "on_file_write",
		Event:   FileWrite,
		Enabled: true,
		Handler: func(e Event) error {
			path, _ := e.Data["path"].(string)
			fn(path)
			return nil
		},
	})
}

// OnError registers a convenience hook that fires on ErrorOccurred events.
func (eb *EventBus) OnError(fn func(err error)) {
	eb.Register(&LifecycleHook{
		ID:      fmt.Sprintf("on_error_%p", fn),
		Name:    "on_error",
		Event:   ErrorOccurred,
		Enabled: true,
		Handler: func(e Event) error {
			if errVal, ok := e.Data["error"].(error); ok {
				fn(errVal)
			} else if msg, ok := e.Data["error"].(string); ok {
				fn(fmt.Errorf("%s", msg))
			}
			return nil
		},
	})
}

// OnSessionEnd registers a convenience hook that fires when a session ends.
func (eb *EventBus) OnSessionEnd(fn func(duration time.Duration, tokens int)) {
	eb.Register(&LifecycleHook{
		ID:      fmt.Sprintf("on_session_end_%p", fn),
		Name:    "on_session_end",
		Event:   SessionEnd,
		Enabled: true,
		Handler: func(e Event) error {
			var dur time.Duration
			var tokens int
			if d, ok := e.Data["duration"].(time.Duration); ok {
				dur = d
			}
			if t, ok := e.Data["tokens"].(int); ok {
				tokens = t
			}
			fn(dur, tokens)
			return nil
		},
	})
}

// OnToolCall registers a convenience hook that fires when a tool call completes.
func (eb *EventBus) OnToolCall(fn func(tool string, duration time.Duration)) {
	eb.Register(&LifecycleHook{
		ID:      fmt.Sprintf("on_tool_call_%p", fn),
		Name:    "on_tool_call",
		Event:   ToolCallEnd,
		Enabled: true,
		Handler: func(e Event) error {
			tool, _ := e.Data["tool"].(string)
			dur, _ := e.Data["duration"].(time.Duration)
			fn(tool, dur)
			return nil
		},
	})
}

// GetHistory returns the most recent events of the given type, limited to `limit`.
// If eventType is empty, all events are considered.
func (eb *EventBus) GetHistory(eventType string, limit int) []Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var matching []Event
	for i := len(eb.History) - 1; i >= 0; i-- {
		if eventType == "" || eb.History[i].Name == eventType {
			matching = append(matching, eb.History[i])
			if limit > 0 && len(matching) >= limit {
				break
			}
		}
	}

	// Reverse so that oldest comes first.
	for i, j := 0, len(matching)-1; i < j; i, j = i+1, j-1 {
		matching[i], matching[j] = matching[j], matching[i]
	}
	return matching
}

// FormatEvent returns a human-readable log line for the event.
func FormatEvent(event Event) string {
	ts := event.Timestamp.Format("15:04:05.000")
	source := event.Source
	if source == "" {
		source = "system"
	}
	dataStr := ""
	if len(event.Data) > 0 {
		parts := make([]string, 0, len(event.Data))
		for k, v := range event.Data {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		dataStr = " " + joinStrings(parts, " ")
	}
	return fmt.Sprintf("[%s] %s (%s)%s", ts, event.Name, source, dataStr)
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// Stats returns aggregate statistics about the event bus.
func (eb *EventBus) Stats() EventStats {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	byType := make(map[string]int)
	for _, e := range eb.History {
		byType[e.Name]++
	}

	hookCount := 0
	asyncCount := 0
	for _, hooks := range eb.Hooks {
		for _, h := range hooks {
			hookCount++
			if h.Async {
				asyncCount++
			}
		}
	}

	var avgTime time.Duration
	if eb.hookCallCount > 0 {
		avgTime = time.Duration(int64(eb.hookTimeTotal) / eb.hookCallCount)
	}

	return EventStats{
		TotalEvents: len(eb.History),
		ByType:      byType,
		HookCount:   hookCount,
		AsyncHooks:  asyncCount,
		AvgHookTime: avgTime,
	}
}
