package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// newPlanModeSession builds a session whose registry includes the plan tools
// and a write tool. The permission callback always approves EnterPlanMode; for
// ExitPlanMode it returns approveExit, modeling the user's plan approval gate.
func newPlanModeSession(approveExit bool) (*Session, *int) {
	registry := tool.NewRegistry(
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.EnterPlanModeTool{},
		tool.ExitPlanModeTool{},
	)
	s := NewSession("", "", "test", registry)
	prompts := 0
	s.PermissionFn = func(req PermissionRequest) {
		prompts++
		allow := true
		if req.ToolName == "ExitPlanMode" {
			allow = approveExit
		}
		if req.Response != nil {
			req.Response <- allow
		}
	}
	return s, &prompts
}

func runTool(t *testing.T, s *Session, name string, args map[string]interface{}) toolExecResult {
	t.Helper()
	ch := make(chan StreamEvent, 32)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := s.executeSingleTool(ctx, types.ToolCall{Name: name, ID: "t1", Arguments: args}, ch, 1, "")
	return res
}

func TestPlanMode_EnterFlipsPermissionMode(t *testing.T) {
	s, _ := newPlanModeSession(true)
	if s.Perm.Mode == PermissionModePlan {
		t.Fatal("should not start in plan mode")
	}
	runTool(t, s, "EnterPlanMode", map[string]interface{}{})
	if s.Perm.Mode != PermissionModePlan {
		t.Errorf("expected plan mode after EnterPlanMode, got %q", s.Perm.Mode)
	}
	if s.Mode != PermissionModePlan {
		t.Errorf("session Mode not synced: %q", s.Mode)
	}
}

func TestPlanMode_WriteDenied(t *testing.T) {
	s, _ := newPlanModeSession(true)
	runTool(t, s, "EnterPlanMode", map[string]interface{}{})

	res := runTool(t, s, "Write", map[string]interface{}{
		"file_path": "/tmp/should_not_write.txt",
		"content":   "nope",
	})
	if !res.isErr {
		t.Errorf("expected Write to be denied in plan mode")
	}
	if !strings.Contains(strings.ToLower(res.output), "denied") {
		t.Errorf("expected denial message, got %q", res.output)
	}
}

func TestPlanMode_ExitApprovedSwitchesToBuild(t *testing.T) {
	s, prompts := newPlanModeSession(true)
	runTool(t, s, "EnterPlanMode", map[string]interface{}{})

	res := runTool(t, s, "ExitPlanMode", map[string]interface{}{})
	if res.isErr {
		t.Errorf("approved ExitPlanMode should not be an error: %q", res.output)
	}
	if s.Perm.Mode != PermissionModeDefault {
		t.Errorf("expected build (default) mode after approval, got %q", s.Perm.Mode)
	}
	if *prompts == 0 {
		t.Errorf("expected an approval prompt on ExitPlanMode")
	}
	if !strings.Contains(strings.ToLower(res.output), "build mode") {
		t.Errorf("expected build-mode confirmation, got %q", res.output)
	}
}

func TestPlanMode_ExitDeniedStaysInPlan(t *testing.T) {
	s, _ := newPlanModeSession(false)
	runTool(t, s, "EnterPlanMode", map[string]interface{}{})

	res := runTool(t, s, "ExitPlanMode", map[string]interface{}{})
	if !res.isErr {
		t.Errorf("denied ExitPlanMode should report an error result to keep planning")
	}
	if s.Perm.Mode != PermissionModePlan {
		t.Errorf("expected to stay in plan mode after denial, got %q", s.Perm.Mode)
	}
}
