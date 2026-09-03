package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// newSpecModeSession builds a session whose registry includes the spec
// workflow tools plus a write tool. The permission callback always approves
// ApproveImplementation when approveImplement is true, modeling the user's
// spec approval gate.
func newSpecModeSession(approveImplement bool) (*Session, *int) {
	registry := tool.NewRegistry(
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.ProposalTool{},
		tool.SpecifyTool{},
		tool.DesignTool{},
		tool.PlanTool{},
		tool.TasksTool{},
		tool.ApproveImplementationTool{},
		tool.SpecResetTool{},
		tool.ConstitutionTool{},
	)
	s := NewSession("", "", "test", registry)
	prompts := 0
	s.SetPermissionFn(func(req PermissionRequest) {
		prompts++
		allow := true
		if req.ToolName == "ApproveImplementation" {
			allow = approveImplement
		}
		if req.Response != nil {
			req.Response <- allow
		}
	})
	return s, &prompts
}

func runSpecTool(t *testing.T, s *Session, name string, args map[string]interface{}) toolExecResult {
	t.Helper()
	ch := make(chan StreamEvent, 32)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := s.executeSingleTool(ctx, types.ToolCall{Name: name, ID: "t1", Arguments: args}, ch, 1, "")
	if !res.isErr {
		s.PermSvc().AdvanceSpecStage(name)
	}
	return res
}

func ensureTestConstitution(t *testing.T, s *Session) {
	t.Helper()
	slug := s.PermSvc().SpecSlug()
	if slug == "" {
		return
	}
	cwd, _ := os.Getwd()
	dir := filepath.Join(cwd, ".graycode", "specs", slug)
	os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "constitution.md")
	os.WriteFile(path, []byte("## Constitution\n"), 0o600)
}

func TestSpecMode_SpecifyAdvancesStage(t *testing.T) {
	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageSpecify)
	runSpecTool(t, s, "Specify", map[string]interface{}{"title": "test", "spec": "problem statement"})
	if s.PermSvc().SpecStage() != SpecStageSpecify {
		t.Errorf("expected stage Specify after Specify tool, got %v", s.PermSvc().SpecStage())
	}
}

func TestSpecMode_PlanTasksAdvanceStage(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })

	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageProposal)
	runSpecTool(t, s, "Proposal", map[string]interface{}{"title": "test", "proposal": "proposal"})
	ensureTestConstitution(t, s)
	runSpecTool(t, s, "Specify", map[string]interface{}{"title": "test", "spec": "problem statement"})
	runSpecTool(t, s, "Design", map[string]interface{}{"design": "technical design"})
	runSpecTool(t, s, "Plan", map[string]interface{}{"plan": "## Summary\n### Simplicity: using <=3 projects\n### Anti-Abstraction: framework directly\n### Integration-First: contract defined\n### Complexity Tracking\n| Gate | Justification |\n|------|---------------|\n"})
	if s.PermSvc().SpecStage() != SpecStagePlan {
		t.Errorf("expected stage Plan after Plan tool, got %v", s.PermSvc().SpecStage())
	}

	runSpecTool(t, s, "Tasks", map[string]interface{}{"tasks": "task breakdown"})
	if s.PermSvc().SpecStage() != SpecStageTasks {
		t.Errorf("expected stage Tasks after Tasks tool, got %v", s.PermSvc().SpecStage())
	}
}

func TestSpecMode_WriteDeniedMidStage(t *testing.T) {
	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageSpecify)

	res := runSpecTool(t, s, "Write", map[string]interface{}{
		"file_path": "/tmp/should_not_write.txt",
		"content":   "nope",
	})
	if !res.isErr {
		t.Errorf("expected Write to be denied mid spec-stage")
	}
	if !strings.Contains(strings.ToLower(res.output), "spec") {
		t.Errorf("expected spec-gate denial message, got %q", res.output)
	}
}

func TestSpecMode_ReadsUnrestrictedMidStage(t *testing.T) {
	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageSpecify)

	res := runSpecTool(t, s, "Read", map[string]interface{}{"file_path": "/tmp/does_not_exist_but_permission_should_pass.txt"})
	// The read itself may fail (file doesn't exist), but it must not be
	// denied by the permission gate — i.e. it should reach the tool's own
	// execution rather than being blocked at the permission layer.
	if strings.Contains(strings.ToLower(res.output), "spec stage active") {
		t.Errorf("expected reads to be unrestricted during spec stage, got %q", res.output)
	}
}

// TestSpecMode_WriteDeniedEvenAtYOLO is the core anti-bypass regression
// test: the old Plan Mode / Autonomy split let a high trust tier
// short-circuit past Plan Mode's gate entirely. The new single ordered
// gate must not allow that regardless of tier.
func TestSpecMode_WriteDeniedEvenAtYOLO(t *testing.T) {
	s, _ := newSpecModeSession(true)
	s.PermSvc().SetAutonomy(AutonomyYOLO)
	s.PermSvc().SetSpecStage(SpecStageSpecify)

	res := runSpecTool(t, s, "Write", map[string]interface{}{
		"file_path": "/tmp/should_not_write_even_at_yolo.txt",
		"content":   "nope",
	})
	if !res.isErr {
		t.Fatal("expected Write to be denied mid spec-stage even at AutonomyYOLO")
	}
}

func TestSpecMode_ApproveImplementationAlwaysPrompts(t *testing.T) {
	s, prompts := newSpecModeSession(true)
	s.PermSvc().SetAutonomy(AutonomyYOLO)
	s.PermSvc().SetSpecStage(SpecStageTasks)

	res := runSpecTool(t, s, "ApproveImplementation", map[string]interface{}{})
	if res.isErr {
		t.Errorf("approved ApproveImplementation should not be an error: %q", res.output)
	}
	if *prompts == 0 {
		t.Errorf("expected an approval prompt on ApproveImplementation even at AutonomyYOLO")
	}
	if s.PermSvc().SpecStage() != SpecStageImplementing {
		t.Errorf("expected stage Implementing after approval, got %v", s.PermSvc().SpecStage())
	}
	if !strings.Contains(strings.ToLower(res.output), "implementation") {
		t.Errorf("expected implementation confirmation, got %q", res.output)
	}
}

func TestSpecMode_ApproveImplementationDeniedStaysGated(t *testing.T) {
	s, _ := newSpecModeSession(false)
	s.PermSvc().SetSpecStage(SpecStageTasks)

	res := runSpecTool(t, s, "ApproveImplementation", map[string]interface{}{})
	if !res.isErr {
		t.Errorf("denied ApproveImplementation should report an error result to keep the gate closed")
	}
	if s.PermSvc().SpecStage() != SpecStageTasks {
		t.Errorf("expected to stay at Tasks stage after denial, got %v", s.PermSvc().SpecStage())
	}
}

// TestSpecMode_ApprovalPromptShowsSpecContent is the end-to-end check that
// the ApproveImplementation prompt isn't a blind yes/no: it exercises the
// full real wiring (Specify writes a real file via the session's
// ToolContext closures -> PermissionEngine.SpecSlug -> specApprovalSummary)
// and asserts the actual prompt summary contains the written spec content.
func TestSpecMode_ApprovalPromptShowsSpecContent(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageProposal)
	runSpecTool(t, s, "Proposal", map[string]interface{}{"title": "test", "proposal": "proposal"})
	ensureTestConstitution(t, s)
	runSpecTool(t, s, "Specify", map[string]interface{}{"title": "approval preview test", "spec": "unique spec marker xyz123"})
	runSpecTool(t, s, "Design", map[string]interface{}{"design": "design"})
	runSpecTool(t, s, "Plan", map[string]interface{}{"plan": "## Summary\nunique plan marker abc456\n\n### Phase -1: Pre-Implementation Gates\n#### Simplicity Gate (Article VII)\n- [x] Using ≤3 projects?\n- [x] No future-proofing?\n\n#### Anti-Abstraction Gate (Article VIII)\n- [x] Using framework directly?\n- [x] Single model representation?\n\n#### Integration-First Gate (Article IX)\n- [x] Contracts defined?\n- [x] Contract tests written?\n\n### Complexity Tracking\n| Gate | Justification |\n|------|---------------|\n| - | All gates pass |"})
	runSpecTool(t, s, "Tasks", map[string]interface{}{"tasks": "unique tasks marker def789"})

	var lastSummary string
	s.SetPermissionFn(func(req PermissionRequest) {
		if req.ToolName == "ApproveImplementation" {
			lastSummary = req.Summary
		}
		if req.Response != nil {
			req.Response <- true
		}
	})
	runSpecTool(t, s, "ApproveImplementation", map[string]interface{}{})

	if !strings.Contains(lastSummary, "unique spec marker xyz123") {
		t.Errorf("approval summary missing spec content: %q", lastSummary)
	}
	if !strings.Contains(lastSummary, "unique plan marker abc456") {
		t.Errorf("approval summary missing plan content: %q", lastSummary)
	}
	if !strings.Contains(lastSummary, "unique tasks marker def789") {
		t.Errorf("approval summary missing tasks content: %q", lastSummary)
	}
}

func TestSpecMode_ImplementingLiftsGate(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageTasks)
	runSpecTool(t, s, "ApproveImplementation", map[string]interface{}{})

	res := runSpecTool(t, s, "Write", map[string]interface{}{
		"file_path": "spec_mode_implementing_write_test.txt",
		"content":   "ok",
	})
	if res.isErr {
		t.Errorf("expected Write to be permitted once Implementing, got error: %q", res.output)
	}
}

func TestSpecMode_ResetClearsStageAndSlug(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	s, _ := newSpecModeSession(true)
	s.PermSvc().SetSpecStage(SpecStageProposal)
	ensureTestConstitution(t, s)
	runSpecTool(t, s, "Proposal", map[string]interface{}{"title": "reset-test", "proposal": "proposal"})
	ensureTestConstitution(t, s)
	runSpecTool(t, s, "Specify", map[string]interface{}{"title": "reset-test", "spec": "content"})
	if s.PermSvc().SpecSlug() == "" {
		t.Fatal("Specify should set an active slug")
	}
	runSpecTool(t, s, "SpecReset", map[string]interface{}{})
	if s.PermSvc().SpecStage() != SpecStageNone || s.PermSvc().SpecSlug() != "" {
		t.Fatalf("reset left stage=%v slug=%q", s.PermSvc().SpecStage(), s.PermSvc().SpecSlug())
	}
}
