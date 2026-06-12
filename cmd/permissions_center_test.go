package cmd

import (
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestNormalizePermissionTier(t *testing.T) {
	level, label, ok := normalizePermissionTier("operator")
	if !ok || level != engine.AutonomyFull || label != "Operator" {
		t.Fatalf("operator = (%v, %q, %v)", level, label, ok)
	}
	level, label, ok = normalizePermissionTier("auto")
	if !ok || level != engine.AutonomyYOLO || label != "Autonomous" {
		t.Fatalf("auto = (%v, %q, %v)", level, label, ok)
	}
}

func TestNormalizePermissionSandbox(t *testing.T) {
	mode, label, ok := normalizePermissionSandbox("workspace")
	if !ok || mode != "workspace" || label != "Workspace" {
		t.Fatalf("workspace = (%q, %q, %v)", mode, label, ok)
	}
	if _, _, ok := normalizePermissionSandbox("ghost"); ok {
		t.Fatal("expected invalid sandbox to fail")
	}
}

func TestEffectivePermissionRules(t *testing.T) {
	settings := hawkconfig.Settings{
		AutoAllow:       []string{"Read"},
		AllowedTools:    []string{"Bash(git:*)", "Read"},
		DisallowedTools: []string{"Bash(rm -rf *)"},
	}
	allow := effectiveAllowRules(settings)
	deny := effectiveDenyRules(settings)
	if len(allow) != 2 {
		t.Fatalf("allow len = %d, want 2 (%v)", len(allow), allow)
	}
	if len(deny) != 1 || deny[0] != "Bash(rm -rf *)" {
		t.Fatalf("deny = %v", deny)
	}
}

func TestPermissionCenterSummary(t *testing.T) {
	perm := engine.NewPermissionEngine()
	perm.Mode = engine.PermissionModeAcceptEdits
	model := &chatModel{
		session: &engine.Session{Autonomy: engine.AutonomySemi, Mode: engine.PermissionModeAcceptEdits, Perm: perm},
		settings: hawkconfig.Settings{
			Sandbox:         "workspace",
			AllowedTools:    []string{"Bash(git:*)"},
			DisallowedTools: []string{"Bash(rm -rf *)"},
		},
	}
	out := permissionCenterSummary(model)
	for _, fragment := range []string{"Permission Center", "Tier: Builder", "Sandbox: Workspace", "Mode: Auto (Edits Allowed)", "Rules: 1 allow, 1 deny"} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("summary %q missing %q", out, fragment)
		}
	}
}

func TestNormalizePermissionMode(t *testing.T) {
	mode, label, ok := normalizePermissionMode("bypass")
	if !ok || mode != engine.PermissionModeBypassPermissions || label != "Bypass Permissions" {
		t.Fatalf("bypass = (%q, %q, %v)", mode, label, ok)
	}
	mode, _, ok = normalizePermissionMode("plan")
	if !ok || mode != engine.PermissionModePlan {
		t.Fatalf("plan = (%q, %v)", mode, ok)
	}
}
