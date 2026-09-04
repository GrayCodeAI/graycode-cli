package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
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
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
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
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
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

func TestExecuteOne_PreStageShortCircuitsBeforeExecution(t *testing.T) {
	executed := false
	read := &countingExecTool{ran: &executed}
	sess := NewSession("test", "test", "system", tool.NewRegistry(read))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)

	// Register a pre-execute interceptor that denies before the raw tool ever
	// runs, proving the StagePreExecute waterfall is in the ExecuteOne path.
	sess.Tools().Pipeline().Register(tool.StagePreExecute, tool.InterceptFn(func(ctx context.Context, req tool.ToolRequest, res *tool.ToolResult, next func() error) error {
		return tool.ShortCircuitDeny("blocked by test policy")
	}))

	ch := make(chan StreamEvent, 4)
	result := sess.Tools().ExecuteOne(context.Background(), types.ToolCall{ID: "r1", Name: "Read", Arguments: map[string]interface{}{"id": 1}}, nil, ch, 0, "test")

	if executed {
		t.Fatal("tool executed despite pre-execute short-circuit")
	}
	if !result.isErr {
		t.Fatal("short-circuited result should be an error")
	}
	if !strings.Contains(result.output, "blocked by test policy") {
		t.Fatalf("result output = %q, want pipeline deny message", result.output)
	}
}

type countingExecTool struct {
	ran *bool
}

func (t *countingExecTool) Name() string        { return "Read" }
func (t *countingExecTool) Description() string { return "counts executions" }
func (t *countingExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (t *countingExecTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	*t.ran = true
	return "should-not-run", nil
}

func TestExecuteOne_EmptyPipelineStillExecutes(t *testing.T) {
	executed := false
	read := &countingExecTool{ran: &executed}
	sess := NewSession("test", "test", "system", tool.NewRegistry(read))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)

	// No interceptor registered: empty pipeline is a strict pass-through.
	ch := make(chan StreamEvent, 4)
	_ = sess.Tools().ExecuteOne(context.Background(), types.ToolCall{ID: "r1", Name: "Read", Arguments: map[string]interface{}{"id": 1}}, nil, ch, 0, "test")
	if !executed {
		t.Fatal("tool did not execute with an empty pipeline")
	}
}

func TestExecuteOne_PreStageErrorIsDistinct(t *testing.T) {
	executed := false
	read := &countingExecTool{ran: &executed}
	sess := NewSession("test", "test", "system", tool.NewRegistry(read))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.Tools().Pipeline().Register(tool.StagePreExecute, tool.InterceptFn(func(ctx context.Context, req tool.ToolRequest, res *tool.ToolResult, next func() error) error {
		return errors.New("pipeline infra failure")
	}))
	ch := make(chan StreamEvent, 4)
	result := sess.Tools().ExecuteOne(context.Background(), types.ToolCall{ID: "r1", Name: "Read", Arguments: map[string]interface{}{"id": 1}}, nil, ch, 0, "test")
	if executed {
		t.Fatal("tool executed despite pre-execute non-ShortCircuit error")
	}
	if !result.isErr {
		t.Fatal("expected error result on non-ShortCircuit pipeline error")
	}
}

type contextCaptureTool struct{ ctx *tool.ToolContext }

func (t *contextCaptureTool) Name() string        { return "Read" }
func (t *contextCaptureTool) Description() string { return "capture context" }
func (t *contextCaptureTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (t *contextCaptureTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	t.ctx = tool.GetToolContext(ctx)
	return "ok", nil
}

func TestExecuteSingleTool_PropagatesPermissionContext(t *testing.T) {
	capture := &contextCaptureTool{}
	sess := NewSession("test", "test", "system", tool.NewRegistry(capture))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.PermSvc().SetSandboxMode(sandbox.ModeOff)
	sess.SetAllowedDirs([]string{"/tmp/extra"})
	ch := make(chan StreamEvent, 4)
	res := sess.executeSingleTool(context.Background(), types.ToolCall{Name: "Read", ID: "ctx"}, ch, 0, "")
	if res.isErr || capture.ctx == nil {
		t.Fatalf("tool failed or context missing: %#v", res)
	}
	if capture.ctx.SandboxMode != sandbox.ModeOff {
		t.Fatalf("SandboxMode = %q, want off", capture.ctx.SandboxMode)
	}
	if len(capture.ctx.AllowedDirectories) != 1 || capture.ctx.AllowedDirectories[0] != "/tmp/extra" {
		t.Fatalf("AllowedDirectories = %#v", capture.ctx.AllowedDirectories)
	}
	if got := sess.PermSvc().AllowedDirs(); len(got) != 1 || got[0] != "/tmp/extra" {
		t.Fatalf("service AllowedDirs = %#v", got)
	}
}

// sandboxModeCaptureTool records ModeFromContext so we can assert the session
// sandbox policy is bridged onto the tool execution context for Bash wrap.
type sandboxModeCaptureTool struct {
	mode sandbox.Mode
}

func (t *sandboxModeCaptureTool) Name() string        { return "Read" }
func (t *sandboxModeCaptureTool) Description() string { return "capture sandbox mode" }
func (t *sandboxModeCaptureTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (t *sandboxModeCaptureTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	t.mode = sandbox.ModeFromContext(ctx)
	return "ok", nil
}

func TestExecuteSingleTool_BridgesSandboxModeOntoContext(t *testing.T) {
	capture := &sandboxModeCaptureTool{}
	sess := NewSession("test", "test", "system", tool.NewRegistry(capture))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.PermSvc().SetSandboxMode(sandbox.ModeWorkspace)
	ch := make(chan StreamEvent, 4)
	res := sess.executeSingleTool(context.Background(), types.ToolCall{Name: "Read", ID: "sb"}, ch, 0, "")
	if res.isErr {
		t.Fatalf("tool failed: %#v", res)
	}
	if capture.mode != sandbox.ModeWorkspace {
		t.Fatalf("ModeFromContext = %q, want %q (session sandbox must wrap shell)", capture.mode, sandbox.ModeWorkspace)
	}
}

func TestExecuteSingleTool_SandboxOffDoesNotSetModeOnContext(t *testing.T) {
	capture := &sandboxModeCaptureTool{}
	sess := NewSession("test", "test", "system", tool.NewRegistry(capture))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.PermSvc().SetSandboxMode(sandbox.ModeOff)
	ch := make(chan StreamEvent, 4)
	res := sess.executeSingleTool(context.Background(), types.ToolCall{Name: "Read", ID: "sb-off"}, ch, 0, "")
	if res.isErr {
		t.Fatalf("tool failed: %#v", res)
	}
	// ModeOff / unset should leave ModeFromContext as ModeOff so host shell
	// is not force-wrapped without a backend.
	if capture.mode != sandbox.ModeOff {
		t.Fatalf("ModeFromContext = %q, want off", capture.mode)
	}
}

func TestGenerateDiffSummary_IncludesUnifiedPreview(t *testing.T) {
	old := "line1\nline2\n"
	newC := "line1\nline2 changed\nline3\n"
	got := generateDiffSummary(old, newC, "demo.go")
	if got == "" {
		t.Fatal("expected non-empty diff summary")
	}
	if !strings.Contains(got, "diff demo.go:") {
		t.Fatalf("missing stats header: %q", got)
	}
	if !strings.Contains(got, "--- a/demo.go") || !strings.Contains(got, "+++ b/demo.go") {
		t.Fatalf("missing unified diff headers: %q", got)
	}
	if !strings.Contains(got, "+line2 changed") && !strings.Contains(got, "+line3") {
		t.Fatalf("missing added lines in preview: %q", got)
	}
}

func TestToolServiceWorkingDirPropagatesContext(t *testing.T) {
	capture := &contextCaptureTool{}
	sess := NewSession("test", "test", "system", tool.NewRegistry(capture))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.Tools().SetWorkingDir("/tmp/graycode-working-dir")

	ch := make(chan StreamEvent, 4)
	res := sess.executeSingleTool(context.Background(), types.ToolCall{Name: "Read", ID: "cwd"}, ch, 0, "")
	if res.isErr || capture.ctx == nil {
		t.Fatalf("tool failed or context missing: %#v", res)
	}
	if capture.ctx.WorkingDir != "/tmp/graycode-working-dir" {
		t.Fatalf("WorkingDir = %q, want %q", capture.ctx.WorkingDir, "/tmp/graycode-working-dir")
	}
}

func TestToolServiceReadOnlyBashPropagatesContext(t *testing.T) {
	capture := &contextCaptureTool{}
	sess := NewSession("test", "test", "system", tool.NewRegistry(capture))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.Tools().SetReadOnlyBash(true)

	ch := make(chan StreamEvent, 4)
	res := sess.executeSingleTool(context.Background(), types.ToolCall{Name: "Read", ID: "readonly"}, ch, 0, "")
	if res.isErr || capture.ctx == nil {
		t.Fatalf("tool failed or context missing: %#v", res)
	}
	if !capture.ctx.ReadOnlyBash {
		t.Fatal("ReadOnlyBash = false, want true")
	}
}
