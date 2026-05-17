package hooks

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewEventBus(t *testing.T) {
	eb := NewEventBus()
	if eb == nil {
		t.Fatal("NewEventBus returned nil")
	}
	if eb.MaxHistory != 1000 {
		t.Fatalf("expected MaxHistory=1000, got %d", eb.MaxHistory)
	}
	if len(eb.Hooks) != 0 {
		t.Fatal("expected empty hooks map")
	}
	if len(eb.Listeners) != 0 {
		t.Fatal("expected empty listeners map")
	}
}

func TestRegisterAndEmit(t *testing.T) {
	eb := NewEventBus()
	var called bool
	eb.Register(&LifecycleHook{
		ID:      "h1",
		Name:    "test_hook",
		Event:   SessionStart,
		Enabled: true,
		Handler: func(e Event) error {
			called = true
			if e.Name != SessionStart {
				t.Errorf("expected event name %s, got %s", SessionStart, e.Name)
			}
			return nil
		},
	})

	eb.Emit(Event{Name: SessionStart, Data: map[string]interface{}{"key": "value"}})
	if !called {
		t.Fatal("hook was not called")
	}
}

func TestRegisterNilHook(t *testing.T) {
	eb := NewEventBus()
	eb.Register(nil) // should not panic
}

func TestUnregister(t *testing.T) {
	eb := NewEventBus()
	callCount := 0
	eb.Register(&LifecycleHook{
		ID:      "removeme",
		Name:    "removable",
		Event:   TurnStart,
		Enabled: true,
		Handler: func(e Event) error {
			callCount++
			return nil
		},
	})

	eb.Emit(Event{Name: TurnStart})
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	eb.Unregister("removeme")
	eb.Emit(Event{Name: TurnStart})
	if callCount != 1 {
		t.Fatalf("expected still 1 call after unregister, got %d", callCount)
	}
}

func TestPriorityOrder(t *testing.T) {
	eb := NewEventBus()
	var order []int

	eb.Register(&LifecycleHook{
		ID:       "low",
		Name:     "low_priority",
		Event:    TurnEnd,
		Priority: 100,
		Enabled:  true,
		Handler: func(e Event) error {
			order = append(order, 100)
			return nil
		},
	})
	eb.Register(&LifecycleHook{
		ID:       "high",
		Name:     "high_priority",
		Event:    TurnEnd,
		Priority: 1,
		Enabled:  true,
		Handler: func(e Event) error {
			order = append(order, 1)
			return nil
		},
	})
	eb.Register(&LifecycleHook{
		ID:       "mid",
		Name:     "mid_priority",
		Event:    TurnEnd,
		Priority: 50,
		Enabled:  true,
		Handler: func(e Event) error {
			order = append(order, 50)
			return nil
		},
	})

	eb.Emit(Event{Name: TurnEnd})
	if len(order) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(order))
	}
	if order[0] != 1 || order[1] != 50 || order[2] != 100 {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestDisabledHook(t *testing.T) {
	eb := NewEventBus()
	called := false
	eb.Register(&LifecycleHook{
		ID:      "disabled",
		Name:    "disabled_hook",
		Event:   FileRead,
		Enabled: false,
		Handler: func(e Event) error {
			called = true
			return nil
		},
	})

	eb.Emit(Event{Name: FileRead})
	if called {
		t.Fatal("disabled hook should not be called")
	}
}

func TestAsyncHook(t *testing.T) {
	eb := NewEventBus()
	var wg sync.WaitGroup
	wg.Add(1)
	var asyncCalled int32

	eb.Register(&LifecycleHook{
		ID:      "async1",
		Name:    "async_hook",
		Event:   ToolCallStart,
		Async:   true,
		Enabled: true,
		Handler: func(e Event) error {
			atomic.AddInt32(&asyncCalled, 1)
			wg.Done()
			return nil
		},
	})

	eb.Emit(Event{Name: ToolCallStart})
	wg.Wait()

	if atomic.LoadInt32(&asyncCalled) != 1 {
		t.Fatal("async hook was not called")
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	eb := NewEventBus()
	ch := eb.Subscribe(FileWrite)

	eb.Emit(Event{Name: FileWrite, Data: map[string]interface{}{"path": "/tmp/test.txt"}})

	select {
	case e := <-ch:
		if e.Name != FileWrite {
			t.Fatalf("expected %s, got %s", FileWrite, e.Name)
		}
		path, _ := e.Data["path"].(string)
		if path != "/tmp/test.txt" {
			t.Fatalf("expected path /tmp/test.txt, got %s", path)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event on channel")
	}

	eb.Unsubscribe(FileWrite, ch)
	eb.Emit(Event{Name: FileWrite, Data: map[string]interface{}{"path": "/tmp/test2.txt"}})

	select {
	case <-ch:
		t.Fatal("should not receive after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestOnFileWrite(t *testing.T) {
	eb := NewEventBus()
	var receivedPath string
	eb.OnFileWrite(func(path string) {
		receivedPath = path
	})

	eb.Emit(Event{Name: FileWrite, Data: map[string]interface{}{"path": "/foo/bar.go"}})
	if receivedPath != "/foo/bar.go" {
		t.Fatalf("expected /foo/bar.go, got %s", receivedPath)
	}
}

func TestOnError(t *testing.T) {
	eb := NewEventBus()
	var receivedErr error

	eb.OnError(func(err error) {
		receivedErr = err
	})

	eb.Emit(Event{Name: ErrorOccurred, Data: map[string]interface{}{"error": fmt.Errorf("something broke")}})
	if receivedErr == nil || receivedErr.Error() != "something broke" {
		t.Fatalf("expected 'something broke', got %v", receivedErr)
	}
}

func TestOnErrorWithString(t *testing.T) {
	eb := NewEventBus()
	var receivedErr error

	eb.OnError(func(err error) {
		receivedErr = err
	})

	eb.Emit(Event{Name: ErrorOccurred, Data: map[string]interface{}{"error": "string error"}})
	if receivedErr == nil || receivedErr.Error() != "string error" {
		t.Fatalf("expected 'string error', got %v", receivedErr)
	}
}

func TestOnSessionEnd(t *testing.T) {
	eb := NewEventBus()
	var gotDuration time.Duration
	var gotTokens int

	eb.OnSessionEnd(func(duration time.Duration, tokens int) {
		gotDuration = duration
		gotTokens = tokens
	})

	eb.Emit(Event{
		Name: SessionEnd,
		Data: map[string]interface{}{
			"duration": 5 * time.Minute,
			"tokens":   15000,
		},
	})

	if gotDuration != 5*time.Minute {
		t.Fatalf("expected 5m, got %v", gotDuration)
	}
	if gotTokens != 15000 {
		t.Fatalf("expected 15000 tokens, got %d", gotTokens)
	}
}

func TestOnToolCall(t *testing.T) {
	eb := NewEventBus()
	var gotTool string
	var gotDuration time.Duration

	eb.OnToolCall(func(tool string, duration time.Duration) {
		gotTool = tool
		gotDuration = duration
	})

	eb.Emit(Event{
		Name: ToolCallEnd,
		Data: map[string]interface{}{
			"tool":     "file_read",
			"duration": 200 * time.Millisecond,
		},
	})

	if gotTool != "file_read" {
		t.Fatalf("expected file_read, got %s", gotTool)
	}
	if gotDuration != 200*time.Millisecond {
		t.Fatalf("expected 200ms, got %v", gotDuration)
	}
}

func TestGetHistory(t *testing.T) {
	eb := NewEventBus()

	for i := 0; i < 10; i++ {
		eb.Emit(Event{Name: FileWrite, Data: map[string]interface{}{"i": i}})
	}
	for i := 0; i < 5; i++ {
		eb.Emit(Event{Name: FileRead, Data: map[string]interface{}{"i": i}})
	}

	// Get all FileWrite events
	writes := eb.GetHistory(FileWrite, 0)
	if len(writes) != 10 {
		t.Fatalf("expected 10 write events, got %d", len(writes))
	}

	// Get limited
	limited := eb.GetHistory(FileWrite, 3)
	if len(limited) != 3 {
		t.Fatalf("expected 3 events, got %d", len(limited))
	}
	// Should be the 3 most recent
	if limited[2].Data["i"] != 9 {
		t.Fatalf("expected last event i=9, got %v", limited[2].Data["i"])
	}

	// Get all events regardless of type
	all := eb.GetHistory("", 0)
	if len(all) != 15 {
		t.Fatalf("expected 15 total events, got %d", len(all))
	}
}

func TestGetHistoryEmpty(t *testing.T) {
	eb := NewEventBus()
	events := eb.GetHistory(SessionStart, 10)
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestMaxHistory(t *testing.T) {
	eb := NewEventBus()
	eb.MaxHistory = 20

	for i := 0; i < 50; i++ {
		eb.Emit(Event{Name: FileWrite, Data: map[string]interface{}{"i": i}})
	}

	eb.mu.RLock()
	histLen := len(eb.History)
	eb.mu.RUnlock()

	if histLen > 20 {
		t.Fatalf("history length %d exceeds max %d", histLen, 20)
	}
}

func TestFormatEvent(t *testing.T) {
	ts := time.Date(2026, 5, 12, 14, 30, 45, 123000000, time.UTC)
	e := Event{
		Name:      SessionStart,
		Timestamp: ts,
		Source:    "engine",
		Data:      map[string]interface{}{"user": "alice"},
	}

	formatted := FormatEvent(e)
	if formatted == "" {
		t.Fatal("FormatEvent returned empty string")
	}
	// Check it contains expected parts
	if !containsStr(formatted, "14:30:45.123") {
		t.Fatalf("expected timestamp in output: %s", formatted)
	}
	if !containsStr(formatted, SessionStart) {
		t.Fatalf("expected event name in output: %s", formatted)
	}
	if !containsStr(formatted, "engine") {
		t.Fatalf("expected source in output: %s", formatted)
	}
}

func TestFormatEventNoSource(t *testing.T) {
	e := Event{
		Name:      ErrorOccurred,
		Timestamp: time.Now(),
	}
	formatted := FormatEvent(e)
	if !containsStr(formatted, "system") {
		t.Fatalf("expected default source 'system' in output: %s", formatted)
	}
}

func TestStats(t *testing.T) {
	eb := NewEventBus()

	eb.Register(&LifecycleHook{
		ID:      "s1",
		Name:    "sync_hook",
		Event:   FileWrite,
		Enabled: true,
		Handler: func(e Event) error { return nil },
	})
	eb.Register(&LifecycleHook{
		ID:      "a1",
		Name:    "async_hook",
		Event:   FileRead,
		Async:   true,
		Enabled: true,
		Handler: func(e Event) error { return nil },
	})

	eb.Emit(Event{Name: FileWrite})
	eb.Emit(Event{Name: FileWrite})
	eb.Emit(Event{Name: FileRead})

	// Give async hook time to complete
	time.Sleep(50 * time.Millisecond)

	stats := eb.Stats()
	if stats.TotalEvents != 3 {
		t.Fatalf("expected 3 total events, got %d", stats.TotalEvents)
	}
	if stats.ByType[FileWrite] != 2 {
		t.Fatalf("expected 2 FileWrite events, got %d", stats.ByType[FileWrite])
	}
	if stats.ByType[FileRead] != 1 {
		t.Fatalf("expected 1 FileRead event, got %d", stats.ByType[FileRead])
	}
	if stats.HookCount != 2 {
		t.Fatalf("expected 2 hooks, got %d", stats.HookCount)
	}
	if stats.AsyncHooks != 1 {
		t.Fatalf("expected 1 async hook, got %d", stats.AsyncHooks)
	}
	if stats.AvgHookTime == 0 {
		t.Log("warning: AvgHookTime is 0 (hook ran too fast to measure)")
	}
}

func TestEventConstants(t *testing.T) {
	// Verify all event type constants are unique.
	events := []string{
		SessionStart, SessionEnd,
		TurnStart, TurnEnd,
		ToolCallStart, ToolCallEnd, ToolCallError,
		FileRead, FileWrite, FileEdit, FileDelete,
		CompactionStart, CompactionEnd,
		BudgetWarning, BudgetExceeded,
		ErrorOccurred, ErrorRecovered,
		ModelSwitch, ProviderSwitch,
		UserInput, AgentResponse,
	}
	seen := make(map[string]bool)
	for _, e := range events {
		if seen[e] {
			t.Fatalf("duplicate event constant: %s", e)
		}
		seen[e] = true
	}
}

func TestConcurrentEmit(t *testing.T) {
	eb := NewEventBus()
	var count int64

	eb.Register(&LifecycleHook{
		ID:      "counter",
		Name:    "counter",
		Event:   UserInput,
		Enabled: true,
		Handler: func(e Event) error {
			atomic.AddInt64(&count, 1)
			return nil
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			eb.Emit(Event{Name: UserInput, Data: map[string]interface{}{"n": n}})
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt64(&count) != 100 {
		t.Fatalf("expected 100 calls, got %d", count)
	}
}

func TestEmitSetsTimestamp(t *testing.T) {
	eb := NewEventBus()
	before := time.Now()
	eb.Emit(Event{Name: AgentResponse})
	after := time.Now()

	history := eb.GetHistory(AgentResponse, 1)
	if len(history) != 1 {
		t.Fatal("expected 1 event in history")
	}
	ts := history[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Fatalf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestMultipleListeners(t *testing.T) {
	eb := NewEventBus()
	ch1 := eb.Subscribe(ModelSwitch)
	ch2 := eb.Subscribe(ModelSwitch)

	eb.Emit(Event{Name: ModelSwitch, Data: map[string]interface{}{"model": "gpt-4"}})

	select {
	case e := <-ch1:
		if e.Data["model"] != "gpt-4" {
			t.Fatal("unexpected data on ch1")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout on ch1")
	}

	select {
	case e := <-ch2:
		if e.Data["model"] != "gpt-4" {
			t.Fatal("unexpected data on ch2")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout on ch2")
	}
}

// containsStr checks if s contains substr (avoids importing strings).
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
