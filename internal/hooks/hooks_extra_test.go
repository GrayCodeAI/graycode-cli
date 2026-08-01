package hooks

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRegistry_ExecuteAsync(t *testing.T) {
	r := NewRegistry()
	var called bool
	var mu sync.Mutex

	r.Register(Hook{
		Name:  "test-async",
		Event: "test_event",
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		},
	})

	r.ExecuteAsync(context.Background(), "test_event", map[string]interface{}{"key": "value"})

	// Wait for async execution
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if called {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if !called {
		t.Error("expected hook to be called asynchronously")
	}
	mu.Unlock()
}

func TestRegistry_ExecuteAsync_PreservesValuesAndCanDrain(t *testing.T) {
	type contextKey string
	const key contextKey = "trace"
	r := NewRegistry()
	seen := make(chan string, 1)
	r.Register(Hook{
		Name:  "drainable",
		Event: "drainable_event",
		Fn: func(ctx context.Context, _ map[string]interface{}) error {
			value, _ := ctx.Value(key).(string)
			seen <- value
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key, "trace-123"))
	cancel()
	r.ExecuteAsync(ctx, "drainable_event", nil)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := r.WaitAsync(waitCtx); err != nil {
		t.Fatalf("WaitAsync: %v", err)
	}
	if got := <-seen; got != "trace-123" {
		t.Fatalf("hook context value = %q, want trace-123", got)
	}
}

func TestRegistry_ExecuteAsync_NoHooks(t *testing.T) {
	r := NewRegistry()
	// Should not panic
	r.ExecuteAsync(context.Background(), "nonexistent", nil)
}

func TestRegistry_ExecuteAsyncEnvelope(t *testing.T) {
	r := NewRegistry()
	var called bool
	var mu sync.Mutex

	r.Register(Hook{
		Name:  "test-async-envelope",
		Event: "test_envelope",
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		},
	})

	r.ExecuteAsyncEnvelope(context.Background(), EventEnvelope{
		EventType: "test_envelope",
		Payload:   map[string]interface{}{"key": "value"},
	})

	// Wait for async execution
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if called {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if !called {
		t.Error("expected hook to be called asynchronously")
	}
	mu.Unlock()
}

func TestRegistry_ExecuteAsyncEnvelope_NoHooks(t *testing.T) {
	r := NewRegistry()
	// Should not panic
	r.ExecuteAsyncEnvelope(context.Background(), EventEnvelope{
		EventType: "nonexistent",
		Payload:   nil,
	})
}

func TestRegistry_ExecuteEnvelope(t *testing.T) {
	r := NewRegistry()
	var called bool

	r.Register(Hook{
		Name:  "test-envelope",
		Event: "test_event",
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			called = true
			return nil
		},
	})

	err := r.ExecuteEnvelope(context.Background(), EventEnvelope{
		EventType: "test_event",
		Payload:   map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Errorf("ExecuteEnvelope error: %v", err)
	}
	if !called {
		t.Error("expected hook to be called")
	}
}

func TestRegistry_ExecuteEnvelope_NoHooks(t *testing.T) {
	r := NewRegistry()
	err := r.ExecuteEnvelope(context.Background(), EventEnvelope{
		EventType: "nonexistent",
		Payload:   nil,
	})
	if err != nil {
		t.Errorf("ExecuteEnvelope with no hooks should not error: %v", err)
	}
}

func TestRegistry_ExecuteEnvelope_HookError(t *testing.T) {
	r := NewRegistry()

	r.Register(Hook{
		Name:  "error-hook",
		Event: "test_event",
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			return context.DeadlineExceeded
		},
	})

	err := r.ExecuteEnvelope(context.Background(), EventEnvelope{
		EventType: "test_event",
		Payload:   nil,
	})
	if err == nil {
		t.Error("expected error from failing hook")
	}
}

func TestRegistry_ExecuteEnvelope_FnV1Fallback(t *testing.T) {
	r := NewRegistry()
	var called bool

	r.Register(Hook{
		Name:  "test-v1",
		Event: "test_event",
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			called = true
			return nil
		},
	})

	err := r.ExecuteEnvelope(context.Background(), EventEnvelope{
		EventType: "test_event",
		Payload:   map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Errorf("ExecuteEnvelope error: %v", err)
	}
	if !called {
		t.Error("expected v1 hook to be called")
	}
}

func TestSortHooks(t *testing.T) {
	hooks := []Hook{
		{Name: "low", Priority: 10},
		{Name: "high", Priority: 1},
		{Name: "medium", Priority: 5},
	}
	sortHooks(hooks)
	if hooks[0].Name != "high" {
		t.Errorf("expected high priority first, got %q", hooks[0].Name)
	}
	if hooks[2].Name != "low" {
		t.Errorf("expected low priority last, got %q", hooks[2].Name)
	}
}

func TestSortHooks_Empty(t *testing.T) {
	hooks := []Hook{}
	sortHooks(hooks)
	if len(hooks) != 0 {
		t.Error("expected empty slice")
	}
}

func TestGlobalRegister(t *testing.T) {
	// Test global registry
	Register(Hook{
		Name:  "global-test",
		Event: "global_test",
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			return nil
		},
	})

	global.Execute(context.Background(), "global_test", nil)

	// Reset global registry
	ResetDecisionHooks()
}

// --- Package-level function tests ---

func TestPackageLevel_ExecuteAsync(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	var called bool
	var mu sync.Mutex

	Register(Hook{
		Name:  "pkg-async",
		Event: "pkg_async_test",
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		},
	})

	ExecuteAsync(context.Background(), "pkg_async_test", nil)

	// Wait for async execution
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if called {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if !called {
		t.Error("expected hook to be called via package-level ExecuteAsync")
	}
	mu.Unlock()
}

func TestPackageLevel_ExecuteEnvelope(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	var called bool
	Register(Hook{
		Name:  "pkg-envelope",
		Event: "pkg_envelope_test",
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			called = true
			return nil
		},
	})

	err := ExecuteEnvelope(context.Background(), EventEnvelope{
		EventType: "pkg_envelope_test",
		Payload:   nil,
	})
	if err != nil {
		t.Errorf("ExecuteEnvelope error: %v", err)
	}
	if !called {
		t.Error("expected hook to be called via package-level ExecuteEnvelope")
	}
}

func TestPackageLevel_ExecuteAsyncEnvelope(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()

	var called bool
	var mu sync.Mutex

	Register(Hook{
		Name:  "pkg-async-envelope",
		Event: "pkg_async_env_test",
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		},
	})

	ExecuteAsyncEnvelope(context.Background(), EventEnvelope{
		EventType: "pkg_async_env_test",
		Payload:   nil,
	})

	// Wait for async execution
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if called {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if !called {
		t.Error("expected hook to be called via package-level ExecuteAsyncEnvelope")
	}
	mu.Unlock()
}

func TestPackageLevel_ExecuteAsync_NoHooks(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()
	// Should not panic
	ExecuteAsync(context.Background(), "nonexistent", nil)
}

func TestPackageLevel_ExecuteEnvelope_NoHooks(t *testing.T) {
	ResetDecisionHooks()
	defer ResetDecisionHooks()
	err := ExecuteEnvelope(context.Background(), EventEnvelope{
		EventType: "nonexistent",
		Payload:   nil,
	})
	if err != nil {
		t.Errorf("ExecuteEnvelope with no hooks should not error: %v", err)
	}
}
