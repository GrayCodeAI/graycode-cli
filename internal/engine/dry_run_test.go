package engine

import (
	"context"
	"testing"
)

// TestDryRun_DeniesEverythingRegardlessOfTierOrStage covers the dry-run
// kill switch that replaced PermissionModeDontAsk: it must deny every tool
// call unconditionally, even at the most permissive autonomy tier and even
// when a spec-workflow tool would otherwise always be allowed.
func TestDryRun_DeniesEverythingRegardlessOfTierOrStage(t *testing.T) {
	sess := newTestSession()
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.PermSvc().SetDryRun(true)

	granted, deny := sess.PermSvc().CheckTool(context.Background(), ToolCallInfo{Name: "Read", Args: map[string]interface{}{"path": "x.txt"}})
	if granted {
		t.Fatal("dry-run should deny Read even at AutonomyYOLO")
	}
	if deny == "" {
		t.Fatal("expected a dry-run deny reason")
	}

	// Even spec-workflow tools, which are otherwise always allowed while a
	// stage is active, must be denied under dry-run.
	sess.PermSvc().SetSpecStage(SpecStageSpecify)
	granted, _ = sess.PermSvc().CheckTool(context.Background(), ToolCallInfo{Name: "Specify", Args: map[string]interface{}{}})
	if granted {
		t.Fatal("dry-run should deny Specify even during an active spec stage")
	}
}

func TestDryRun_OffRestoresNormalBehavior(t *testing.T) {
	sess := newTestSession()
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	sess.PermSvc().SetDryRun(true)
	sess.PermSvc().SetDryRun(false)

	granted, _ := sess.PermSvc().CheckTool(context.Background(), ToolCallInfo{Name: "Read", Args: map[string]interface{}{"path": "x.txt"}})
	if !granted {
		t.Fatal("expected Read to be allowed again once dry-run is turned off")
	}
}
