package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

type orderedReadTool struct{}

func (orderedReadTool) Name() string        { return "Read" }
func (orderedReadTool) Description() string { return "test read" }
func (orderedReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "integer"}}}
}

func (orderedReadTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ID    int `json:"id"`
		Delay int `json:"delay"`
	}
	_ = json.Unmarshal(input, &p)
	if p.Delay > 0 {
		select {
		case <-time.After(time.Duration(p.Delay) * time.Millisecond):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return fmt.Sprintf("result-%d", p.ID), nil
}

type countedReadTool struct {
	current int32
	max     int32
}

func (countedReadTool) Name() string        { return "Read" }
func (countedReadTool) Description() string { return "test read" }
func (countedReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "integer"}}}
}

func (t *countedReadTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	cur := atomic.AddInt32(&t.current, 1)
	for {
		max := atomic.LoadInt32(&t.max)
		if cur <= max || atomic.CompareAndSwapInt32(&t.max, max, cur) {
			break
		}
	}
	defer atomic.AddInt32(&t.current, -1)
	select {
	case <-time.After(20 * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return "ok", nil
}

func TestExecuteToolCalls_PreservesOriginalOrder(t *testing.T) {
	sess := NewSession("test", "test", "system", tool.NewRegistry(orderedReadTool{}))
	if err := sess.SetPermissionMode("bypassPermissions"); err != nil {
		t.Fatal(err)
	}
	calls := []types.ToolCall{
		{ID: "slow", Name: "Read", Arguments: map[string]interface{}{"id": 1, "delay": 30}},
		{ID: "fast", Name: "Read", Arguments: map[string]interface{}{"id": 2, "delay": 1}},
	}
	ch := make(chan StreamEvent, 16)

	results := sess.executeToolCalls(context.Background(), calls, ch, 0, "test")

	if len(results) != 2 {
		t.Fatalf("results length = %d, want 2", len(results))
	}
	if results[0].tc.ID != "slow" || results[0].output != "result-1" {
		t.Fatalf("results[0] = %#v, want slow/result-1", results[0])
	}
	if results[1].tc.ID != "fast" || results[1].output != "result-2" {
		t.Fatalf("results[1] = %#v, want fast/result-2", results[1])
	}
}

func TestExecuteToolCalls_BoundsReadOnlyConcurrency(t *testing.T) {
	read := &countedReadTool{}
	sess := NewSession("test", "test", "system", tool.NewRegistry(read))
	if err := sess.SetPermissionMode("bypassPermissions"); err != nil {
		t.Fatal(err)
	}
	calls := make([]types.ToolCall, maxConcurrentReadOnlyToolCalls+6)
	for i := range calls {
		calls[i] = types.ToolCall{ID: fmt.Sprintf("r%d", i), Name: "Read", Arguments: map[string]interface{}{"id": i}}
	}
	ch := make(chan StreamEvent, len(calls)*2)

	results := sess.executeToolCalls(context.Background(), calls, ch, 0, "test")

	if len(results) != len(calls) {
		t.Fatalf("results length = %d, want %d", len(results), len(calls))
	}
	if got := atomic.LoadInt32(&read.max); got > maxConcurrentReadOnlyToolCalls {
		t.Fatalf("max concurrent read-only calls = %d, want <= %d", got, maxConcurrentReadOnlyToolCalls)
	}
}

var (
	_ tool.Tool = orderedReadTool{}
	_ tool.Tool = (*countedReadTool)(nil)
)
