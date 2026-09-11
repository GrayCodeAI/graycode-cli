package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// timeoutBlockTool blocks until the execution deadline fires, then returns the
// context error — canonical long-running-tool behavior. It declares a 50ms
// budget via tool.TimeoutProvider.
type timeoutBlockTool struct{}

func (timeoutBlockTool) Name() string        { return "TimeoutBlock" }
func (timeoutBlockTool) Description() string { return "blocks until the deadline fires" }
func (timeoutBlockTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (timeoutBlockTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	<-ctx.Done()
	return "partial", ctx.Err()
}
func (timeoutBlockTool) Timeout() time.Duration { return 50 * time.Millisecond }

// fastTool completes immediately and declares no budget: the name-based
// fallback applies, which must not interfere with a quick call.
type fastTool struct{}

func (fastTool) Name() string        { return "Fast" }
func (fastTool) Description() string { return "completes immediately" }
func (fastTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (fastTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	return "done", nil
}

// selfDeadlineTool returns its own DeadlineExceeded while the outer budget is
// still live. It must NOT be labelled TOOL_TIMEOUT — the deadline this call
// imposed on the execution context did not win.
type selfDeadlineTool struct{}

func (selfDeadlineTool) Name() string        { return "SelfDeadline" }
func (selfDeadlineTool) Description() string { return "fails with its own deadline error" }
func (selfDeadlineTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (selfDeadlineTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	return "", context.DeadlineExceeded
}
func (selfDeadlineTool) Timeout() time.Duration { return 5 * time.Second }
func (selfDeadlineTool) RetryPolicy() tool.RetryPolicy {
	// No retries: the tool fails deterministically, so a single attempt keeps
	// the test fast and the error path deterministic.
	return tool.RetryPolicy{MaxRetries: 0}
}

func newTimeoutTestSession(t *testing.T, tools ...tool.Tool) *Session {
	t.Helper()
	sess := NewSession("timeout-test", "test", "system", tool.NewRegistry(tools...))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	return sess
}

func TestExecuteOne_DeclaredBudgetTimeout(t *testing.T) {
	sess := newTimeoutTestSession(t, timeoutBlockTool{})
	ch := make(chan StreamEvent, 4)
	result := sess.Tools().ExecuteOne(context.Background(),
		types.ToolCall{ID: "t1", Name: "TimeoutBlock"}, nil, ch, 0, "test")

	if !result.isErr {
		t.Fatalf("expected error for timed-out tool, got output %q", result.output)
	}
	if !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", result.err)
	}
	if !strings.Contains(result.output, "timed out after 50ms") {
		t.Fatalf("output missing structured timeout message: %q", result.output)
	}
	if !strings.Contains(result.output, "code TOOL_TIMEOUT") {
		t.Fatalf("output missing TOOL_TIMEOUT code: %q", result.output)
	}
}

func TestExecuteOne_NoDeclaredBudgetCompletes(t *testing.T) {
	sess := newTimeoutTestSession(t, fastTool{})
	ch := make(chan StreamEvent, 4)
	result := sess.Tools().ExecuteOne(context.Background(),
		types.ToolCall{ID: "t2", Name: "Fast"}, nil, ch, 0, "test")

	if result.isErr {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if result.output != "done" {
		t.Fatalf("output = %q, want done", result.output)
	}
}

func TestExecuteOne_InternalDeadlineNotLabelledTimeout(t *testing.T) {
	sess := newTimeoutTestSession(t, selfDeadlineTool{})
	ch := make(chan StreamEvent, 4)
	result := sess.Tools().ExecuteOne(context.Background(),
		types.ToolCall{ID: "t3", Name: "SelfDeadline"}, nil, ch, 0, "test")

	if !result.isErr {
		t.Fatalf("expected error, got output %q", result.output)
	}
	if strings.Contains(result.output, "TOOL_TIMEOUT") {
		t.Fatalf("internal deadline error mislabelled TOOL_TIMEOUT: %q", result.output)
	}
	if !strings.Contains(result.output, "context deadline exceeded") {
		t.Fatalf("expected raw deadline error to remain, got: %q", result.output)
	}
}
