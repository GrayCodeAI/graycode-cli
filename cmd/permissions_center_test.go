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

func TestAutonomyCenterSummary(t *testing.T) {
	perm := engine.NewPermissionEngine()
	perm.Autonomy = engine.AutonomySemi
	perm.Stage = engine.SpecStageSpecify
	model := &chatModel{
		session: &engine.Session{Autonomy: engine.AutonomySemi, Perm: perm},
		settings: hawkconfig.Settings{
			Sandbox:         "workspace",
			AllowedTools:    []string{"Bash(git:*)"},
			DisallowedTools: []string{"Bash(rm -rf *)"},
		},
	}
	out := autonomyCenterSummary(model)
	for _, fragment := range []string{"Autonomy Center", "Tier: Builder", "Sandbox: Workspace", "Spec stage: Specify", "Rules: 1 allow, 1 deny"} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("summary %q missing %q", out, fragment)
		}
	}
}
