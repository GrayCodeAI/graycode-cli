package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleDiscoverer_FindsAgentsMd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Root Rules"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	found := false
	for _, r := range rules {
		if r.Source == "AGENTS.md" {
			found = true
			if r.Content != "# Root Rules" {
				t.Errorf("content mismatch: got %q", r.Content)
			}
		}
	}
	if !found {
		t.Error("AGENTS.md not found")
	}
}

func TestRuleDiscoverer_Precedence(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Root"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("# Src"), 0o644)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	if len(rules) < 2 {
		t.Fatalf("expected at least 2 rules, got %d", len(rules))
	}
	// Both are local, same source priority — ordered by distance (root=0 first, src=1 second)
	// The root file is closer to the project root (distance 0) so it comes first
	if rules[0].Content != "# Root" {
		t.Errorf("expected root rule first (distance 0), got %q", rules[0].Content)
	}
	if rules[1].Content != "# Src" {
		t.Errorf("expected src rule second (distance 1), got %q", rules[1].Content)
	}
}

func TestRuleDiscoverer_Deduplication(t *testing.T) {
	dir := t.TempDir()
	sameContent := "# Same content everywhere"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(sameContent), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte(sameContent), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(filepath.Join(sub, "main.go"))

	// Same content hash → deduped to 1
	if len(rules) != 1 {
		t.Errorf("expected 1 after dedup, got %d", len(rules))
	}
}

func TestRuleDiscoverer_DirectorySources(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".agents", "rules")
	os.MkdirAll(rulesDir, 0o755)
	os.WriteFile(filepath.Join(rulesDir, "naming.md"), []byte("# Naming Conventions"), 0o644)
	os.WriteFile(filepath.Join(rulesDir, "testing.md"), []byte("# Testing Rules"), 0o644)

	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	found := 0
	for _, r := range rules {
		if r.Source == ".agents/rules" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected 2 rules from .agents/rules, got %d", found)
	}
}

func TestRuleDiscoverer_LocalVsGlobal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Local"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	// Add a fake global rule
	rd.globalDirs = []string{filepath.Join(dir, "global-rules")}
	os.MkdirAll(filepath.Join(dir, "global-rules"), 0o755)
	os.WriteFile(filepath.Join(dir, "global-rules", "base.md"), []byte("# Global"), 0o644)

	rules := rd.Discover(target)

	// Local should come before global
	lastLocal := -1
	firstGlobal := len(rules)
	for i, r := range rules {
		if r.Local {
			lastLocal = i
		} else if firstGlobal == len(rules) {
			firstGlobal = i
		}
	}
	if lastLocal > firstGlobal {
		t.Error("local rules should come before global rules")
	}
}

func TestRuleDiscoverer_SourcePriority(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "a.md"), []byte("# Hawk"), 0o644)
	os.WriteFile(filepath.Join(dir, ".claude", "rules", "b.md"), []byte("# Claude"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	// .agents/rules (priority 1) should come before .claude/rules (priority 3)
	var hawkIdx, claudeIdx int
	for i, r := range rules {
		if r.Source == ".agents/rules" {
			hawkIdx = i
		}
		if r.Source == ".claude/rules" {
			claudeIdx = i
		}
	}
	if hawkIdx >= claudeIdx {
		t.Error(".agents/rules should have higher precedence than .claude/rules")
	}
}

func TestRuleDiscoverer_EmptyProject(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)
	if len(rules) != 0 {
		t.Errorf("expected 0 rules in empty project, got %d", len(rules))
	}
}

func TestRuleDiscoverer_ManagedTierPrecedence(t *testing.T) {
	dir := t.TempDir()
	// A project rule that would normally have top precedence.
	os.WriteFile(filepath.Join(dir, "HAWK.md"), []byte("# Project Policy"), 0o644)
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	// Stand in for the IT-managed policy file (default paths are system-level).
	managed := filepath.Join(dir, "managed-HAWK.md")
	os.WriteFile(managed, []byte("# Org Policy"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rd.managedPaths = []string{managed}

	rules := rd.Discover(target)
	if len(rules) < 2 {
		t.Fatalf("expected at least 2 rules (managed + project), got %d", len(rules))
	}
	if rules[0].Source != managedSource {
		t.Fatalf("expected managed rule first, got source %q content %q", rules[0].Source, rules[0].Content)
	}
	if rules[0].Content != "# Org Policy" {
		t.Errorf("managed content mismatch: got %q", rules[0].Content)
	}
	// Managed must outrank the project HAWK.md regardless of project precedence.
	for i, r := range rules {
		if r.Source == "HAWK.md" && i == 0 {
			t.Error("project HAWK.md should not outrank managed tier")
		}
	}
}

func TestRuleDiscoverer_ManagedTierMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HAWK.md"), []byte("# Project"), 0o644)
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rd.managedPaths = []string{filepath.Join(dir, "does-not-exist.md")}

	rules := rd.Discover(target)
	for _, r := range rules {
		if r.Source == managedSource {
			t.Error("no managed rule should be produced when the file is absent")
		}
	}
}

func TestStripHTMLComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"none", "# Title\nbody", "# Title\nbody"},
		{"inline", "a <!-- secret --> b", "a  b"},
		{"multiline", "head\n<!--\nhidden\nlines\n-->\ntail", "head\n\ntail"},
		{"multiple", "<!--x-->keep<!--y-->", "keep"},
		{"only", "<!-- everything -->", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripHTMLComments(tt.in); got != tt.want {
				t.Errorf("stripHTMLComments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRuleDiscoverer_StripsHTMLCommentsOnLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HAWK.md"), []byte("# Rules\n<!-- internal note: do not ship -->\nUse tabs."), 0o644)
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	found := false
	for _, r := range rules {
		if r.Source == "HAWK.md" {
			found = true
			if strings.Contains(r.Content, "internal note") {
				t.Errorf("HTML comment not stripped from loaded content: %q", r.Content)
			}
			if !strings.Contains(r.Content, "Use tabs.") {
				t.Errorf("non-comment content lost: %q", r.Content)
			}
		}
	}
	if !found {
		t.Fatal("HAWK.md rule not loaded")
	}
}

func TestRuleDiscoverer_NestedDirSources(t *testing.T) {
	dir := t.TempDir()
	// Create .github/instructions at root
	ghDir := filepath.Join(dir, ".github", "instructions")
	os.MkdirAll(ghDir, 0o755)
	os.WriteFile(filepath.Join(ghDir, "pr.md"), []byte("# PR Guidelines"), 0o644)

	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	found := false
	for _, r := range rules {
		if r.Source == ".github/instructions" {
			found = true
		}
	}
	if !found {
		t.Error("expected rules from .github/instructions")
	}
}
