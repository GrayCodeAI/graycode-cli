package hooks

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteEnvelope_FnV2(t *testing.T) {
	r := NewRegistry()
	var got EventEnvelope
	r.Register(Hook{
		Name:  "v2",
		Event: EventPreTool,
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			got = env
			return nil
		},
	})

	env := EventEnvelope{
		Source:        "test",
		SessionID:     "sess-1",
		AgentID:       "agent-1",
		CorrelationID: "corr-1",
		EventType:     EventPreTool,
		Payload:       map[string]interface{}{"tool": "Read"},
	}
	if err := r.ExecuteEnvelope(context.Background(), env); err != nil {
		t.Fatalf("ExecuteEnvelope: %v", err)
	}
	if got.SessionID != "sess-1" || got.AgentID != "agent-1" || got.CorrelationID != "corr-1" {
		t.Errorf("envelope metadata not passed through: %+v", got)
	}
	if got.Payload["tool"] != "Read" {
		t.Errorf("payload not passed through: %+v", got.Payload)
	}
}

func TestExecuteEnvelope_LegacyCompatibility(t *testing.T) {
	r := NewRegistry()
	var gotData map[string]interface{}
	r.Register(Hook{
		Name:  "legacy",
		Event: EventPostTool,
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			gotData = data
			return nil
		},
	})

	env := EventEnvelope{
		EventType: EventPostTool,
		Payload:   map[string]interface{}{"result": "ok"},
	}
	if err := r.ExecuteEnvelope(context.Background(), env); err != nil {
		t.Fatalf("ExecuteEnvelope: %v", err)
	}
	if gotData["result"] != "ok" {
		t.Errorf("legacy hook did not receive payload: %+v", gotData)
	}
}

func TestExecute_BuildsEnvelope(t *testing.T) {
	r := NewRegistry()
	var got EventEnvelope
	r.Register(Hook{
		Name:  "v2",
		Event: EventSessionStart,
		FnV2: func(ctx context.Context, env EventEnvelope) error {
			got = env
			return nil
		},
	})

	data := map[string]interface{}{"id": "x"}
	if err := r.Execute(context.Background(), EventSessionStart, data); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.EventType != EventSessionStart {
		t.Errorf("expected event type %q, got %q", EventSessionStart, got.EventType)
	}
	if got.Source != "hooks" {
		t.Errorf("expected source 'hooks', got %q", got.Source)
	}
	if got.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
	if got.Payload["id"] != "x" {
		t.Errorf("payload not carried: %+v", got.Payload)
	}
}

func TestExecuteEnvelope_MixedHooks(t *testing.T) {
	r := NewRegistry()
	calls := 0
	r.Register(Hook{Name: "legacy", Event: EventError, Fn: func(ctx context.Context, d map[string]interface{}) error {
		calls++
		return nil
	}})
	r.Register(Hook{Name: "v2", Event: EventError, FnV2: func(ctx context.Context, e EventEnvelope) error {
		calls++
		return nil
	}})

	env := EventEnvelope{EventType: EventError, Payload: map[string]interface{}{}}
	_ = r.ExecuteEnvelope(context.Background(), env)
	if calls != 2 {
		t.Errorf("expected both hooks to fire, got %d calls", calls)
	}
}

func TestExecuteEnvelope_FailOpen(t *testing.T) {
	r := NewRegistry()
	second := false
	r.Register(Hook{Name: "fails", Event: EventPreQuery, Priority: 1, FnV2: func(ctx context.Context, e EventEnvelope) error {
		return errors.New("boom")
	}})
	r.Register(Hook{Name: "runs", Event: EventPreQuery, Priority: 2, FnV2: func(ctx context.Context, e EventEnvelope) error {
		second = true
		return nil
	}})

	err := r.ExecuteEnvelope(context.Background(), EventEnvelope{EventType: EventPreQuery})
	if err == nil {
		t.Error("expected first error to be returned")
	}
	if !second {
		t.Error("expected second hook to run despite first failing (fail-open)")
	}
}

func TestAdaptLegacyFn(t *testing.T) {
	var gotData map[string]interface{}
	fn := AdaptLegacyFn(func(ctx context.Context, data map[string]interface{}) error {
		gotData = data
		return nil
	})
	env := EventEnvelope{Payload: map[string]interface{}{"k": "v"}}
	if err := fn(context.Background(), env); err != nil {
		t.Fatalf("adapted fn: %v", err)
	}
	if gotData["k"] != "v" {
		t.Errorf("adapter did not pass payload: %+v", gotData)
	}
}
