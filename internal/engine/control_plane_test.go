package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestParseIsolationProfile(t *testing.T) {
	p, err := ParseIsolationProfile("workspace")
	if err != nil || p.OSMode != sandbox.ModeWorkspace {
		t.Fatalf("workspace: %#v %v", p, err)
	}
	p, err = ParseIsolationProfile("container")
	if err != nil || !p.ContainerRequired || p.OSMode != sandbox.ModeWorkspace {
		t.Fatalf("container: %#v %v", p, err)
	}
	p, err = ParseIsolationProfile("os=strict,container=true")
	if err != nil || p.OSMode != sandbox.ModeStrict || !p.ContainerRequired {
		t.Fatalf("custom: %#v %v", p, err)
	}
}

func TestApplyIsolationProfile(t *testing.T) {
	sess := NewSession("test", "test", "sys", tool.NewRegistry())
	sess.ApplyIsolationProfile(IsolationWorkspace)
	if sess.PermSvc().SandboxMode() != sandbox.ModeWorkspace {
		t.Fatalf("sandbox mode = %q", sess.PermSvc().SandboxMode())
	}
	if sess.Isolation().Label != "workspace" {
		t.Fatalf("isolation label = %q", sess.Isolation().Label)
	}
	sess.ApplyIsolationProfile(IsolationContainer)
	if !sess.Isolation().ContainerRequired {
		t.Fatal("expected container required")
	}
}

func TestWorkModePlanFiltersToolsAndBash(t *testing.T) {
	reg := tool.NewRegistry(
		tool.BashTool{}, tool.FileReadTool{}, tool.FileWriteTool{}, tool.GrepTool{},
		tool.ToolSearchTool{},
	)
	reg.EnableLazyModelSurface([]string{"Bash", "Read", "Write", "Grep", "ToolSearch"})
	sess := NewSession("test", "test", "sys", reg)
	if err := sess.SetWorkMode(WorkModePlan); err != nil {
		t.Fatal(err)
	}
	if !sess.Tools().ReadOnlyBash() {
		t.Fatal("plan mode should set read-only bash")
	}
	names := reg.ModelVisibleNames()
	for _, n := range names {
		if n == "Write" {
			t.Fatalf("Write should not be model-visible in plan mode: %v", names)
		}
	}
	if !reg.IsModelVisible("Read") {
		t.Fatal("Read should be visible in plan")
	}
	if sess.WorkMode() != WorkModePlan {
		t.Fatalf("WorkMode = %s", sess.WorkMode())
	}
	if addon := sess.workModeSystemAddon(); !strings.Contains(addon, "PLAN") {
		t.Fatalf("plan addon missing: %q", addon)
	}
}

func TestLazyGraycodeRouterToolsAndPromote(t *testing.T) {
	reg := tool.NewRegistry(tool.FileReadTool{}, tool.ImpactTool{})
	reg.EnableLazyModelSurface([]string{"Read"})
	graycodeRouter := reg.GraycodeRouterTools()
	if len(graycodeRouter) != 1 || graycodeRouter[0].Name != "Read" {
		t.Fatalf("GraycodeRouterTools = %#v, want only Read", graycodeRouter)
	}
	if !reg.PromoteModelTool("Impact") {
		t.Fatal("promote Impact failed")
	}
	graycodeRouter = reg.GraycodeRouterTools()
	if len(graycodeRouter) != 2 {
		t.Fatalf("after promote GraycodeRouterTools len = %d", len(graycodeRouter))
	}
}

func TestToolSearchSelectPromotes(t *testing.T) {
	reg := tool.NewRegistry(tool.FileReadTool{}, tool.ImpactTool{}, tool.ToolSearchTool{})
	reg.EnableLazyModelSurface([]string{"Read", "ToolSearch"})
	sess := NewSession("test", "test", "sys", reg)
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	ch := make(chan StreamEvent, 8)
	input, _ := json.Marshal(map[string]interface{}{"query": "select:Impact"})
	res := sess.executeSingleTool(context.Background(), types.ToolCall{
		Name: "ToolSearch", ID: "ts1",
		Arguments: map[string]interface{}{"query": "select:Impact"},
	}, ch, 0, "")
	_ = input
	if res.isErr {
		t.Fatalf("ToolSearch failed: %v", res.err)
	}
	if !reg.IsModelVisible("Impact") {
		t.Fatalf("Impact should be promoted; visible=%v", reg.ModelVisibleNames())
	}
}

func TestSpawnControllerStatus(t *testing.T) {
	sess := NewSession("test", "test", "sys", tool.NewRegistry())
	sess.WireAgentTool()
	sc := sess.SpawnController()
	if sc.Status() == "" {
		t.Fatal("empty status")
	}
	if sc.Tasks() == nil {
		t.Fatal("tasks registry nil after ensure")
	}
}
