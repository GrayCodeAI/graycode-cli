package context

import (
	"os"
	"path/filepath"
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
	rulesDir := filepath.Join(dir, ".hawk", "rules")
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
		if r.Source == ".hawk/rules" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected 2 rules from .hawk/rules, got %d", found)
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
	os.MkdirAll(filepath.Join(dir, ".hawk", "rules"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".hawk", "rules", "a.md"), []byte("# Hawk"), 0o644)
	os.WriteFile(filepath.Join(dir, ".claude", "rules", "b.md"), []byte("# Claude"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	target := filepath.Join(sub, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	rd := NewRuleDiscoverer(dir)
	rules := rd.Discover(target)

	// .hawk/rules (priority 1) should come before .claude/rules (priority 3)
	var hawkIdx, claudeIdx int
	for i, r := range rules {
		if r.Source == ".hawk/rules" {
			hawkIdx = i
		}
		if r.Source == ".claude/rules" {
			claudeIdx = i
		}
	}
	if hawkIdx >= claudeIdx {
		t.Error(".hawk/rules should have higher precedence than .claude/rules")
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
