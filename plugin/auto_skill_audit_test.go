package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeProject(t *testing.T) {
	dir := t.TempDir()

	// Create Go project markers.
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM golang"), 0o644)
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:"), 0o644)

	signals := AnalyzeProject(dir)
	found := map[string]bool{}
	for _, s := range signals {
		found[s.Name] = true
	}
	if !found["go"] {
		t.Error("expected 'go' signal")
	}
	if !found["docker"] {
		t.Error("expected 'docker' signal")
	}
	if !found["make"] {
		t.Error("expected 'make' signal")
	}
}

func TestAnalyzeProjectFrameworks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"^18","next":"^14"}}`), 0o644)

	signals := AnalyzeProject(dir)
	found := map[string]bool{}
	for _, s := range signals {
		found[s.Name] = true
	}
	if !found["javascript"] {
		t.Error("expected 'javascript' signal")
	}
	if !found["react"] {
		t.Error("expected 'react' signal")
	}
	if !found["nextjs"] {
		t.Error("expected 'nextjs' signal")
	}
}

func TestAnalyzeProjectEmpty(t *testing.T) {
	dir := t.TempDir()
	signals := AnalyzeProject(dir)
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for empty dir, got %d", len(signals))
	}
}

func TestRecommendSkills(t *testing.T) {
	signals := []ProjectSignal{
		{Category: "language", Name: "go"},
		{Category: "pattern", Name: "docker"},
	}
	skills := []SkillEntry{
		{Name: "go-review", Tags: []string{"go", "review"}, Installs: 100},
		{Name: "docker-deploy", Tags: []string{"docker", "deploy"}, Installs: 200},
		{Name: "react-ui", Tags: []string{"react", "ui"}, Installs: 300},
		{Name: "go-docker", Tags: []string{"go", "docker"}, Installs: 50},
	}

	recommended := RecommendSkills(signals, skills)
	if len(recommended) == 0 {
		t.Fatal("expected recommendations")
	}
	// go-docker should rank highest (matches both signals).
	if recommended[0].Name != "go-docker" {
		t.Errorf("expected go-docker first, got %s", recommended[0].Name)
	}
	// react-ui should not appear.
	for _, r := range recommended {
		if r.Name == "react-ui" {
			t.Error("react-ui should not be recommended for go+docker project")
		}
	}
}

func TestRecommendSkillsNoMatch(t *testing.T) {
	signals := []ProjectSignal{{Category: "language", Name: "haskell"}}
	skills := []SkillEntry{
		{Name: "go-review", Tags: []string{"go"}},
	}
	recommended := RecommendSkills(signals, skills)
	if len(recommended) != 0 {
		t.Errorf("expected 0 recommendations, got %d", len(recommended))
	}
}

func TestAuditCleanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte("# Clean Skill\nNo dangerous characters here."), 0o644)

	findings, err := AuditSkillFile(path)
	if err != nil {
		t.Fatalf("AuditSkillFile: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestAuditBiDiOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	// U+202E is Right-to-Left Override.
	content := "# Skill\nThis has a \u202E hidden override."
	os.WriteFile(path, []byte(content), 0o644)

	findings, err := AuditSkillFile(path)
	if err != nil {
		t.Fatalf("AuditSkillFile: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != SeverityCritical {
		t.Errorf("expected CRITICAL, got %s", findings[0].Severity)
	}
	if findings[0].Category != "bidi-override" {
		t.Errorf("expected bidi-override, got %s", findings[0].Category)
	}
}

func TestAuditZeroWidth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	// U+200B is zero-width space.
	content := "# Skill\nHidden\u200Bspace."
	os.WriteFile(path, []byte(content), 0o644)

	findings, err := AuditSkillFile(path)
	if err != nil {
		t.Fatalf("AuditSkillFile: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != SeverityWarning {
		t.Errorf("expected WARNING, got %s", findings[0].Severity)
	}
}

func TestAuditUnicodeTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	// U+E0001 is a Unicode tag character.
	content := "# Skill\nTag: \U000E0001"
	os.WriteFile(path, []byte(content), 0o644)

	findings, err := AuditSkillFile(path)
	if err != nil {
		t.Fatalf("AuditSkillFile: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != SeverityCritical {
		t.Errorf("expected CRITICAL, got %s", findings[0].Severity)
	}
}

func TestAuditSkillDir(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Clean\nOK."), 0o644)
	os.WriteFile(filepath.Join(skillDir, "notes.md"), []byte("# Notes\nWith \u202E bidi."), 0o644)

	result := AuditSkillDir(dir)
	if result.Files != 2 {
		t.Errorf("expected 2 files scanned, got %d", result.Files)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

func TestFormatAuditResultClean(t *testing.T) {
	r := AuditResult{Files: 3}
	out := FormatAuditResult(r)
	if !strings.Contains(out, "No security issues found") {
		t.Error("expected clean message")
	}
}

func TestFormatAuditResultFindings(t *testing.T) {
	r := AuditResult{
		Files: 1,
		Findings: []AuditFinding{
			{File: "test.md", Line: 1, Column: 5, Severity: SeverityCritical, Category: "bidi-override", Message: "BiDi override (U+202E)"},
		},
	}
	out := FormatAuditResult(r)
	if !strings.Contains(out, "CRITICAL") {
		t.Error("expected CRITICAL in output")
	}
	if !strings.Contains(out, "test.md:1:5") {
		t.Error("expected file location")
	}
}

func TestStripDangerousChars(t *testing.T) {
	input := "Hello\u202E world\u200B end"
	result := StripDangerousChars(input)
	if strings.ContainsRune(result, '\u202E') {
		t.Error("BiDi override should be stripped")
	}
	if strings.ContainsRune(result, '\u200B') {
		t.Error("zero-width space should be stripped")
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "world") {
		t.Error("normal text should be preserved")
	}
}

func TestDefaultSkillDirsCrossAgent(t *testing.T) {
	dirs := DefaultSkillDirs()
	found := map[string]bool{}
	for _, d := range dirs {
		if strings.Contains(d, ".agents/skills") {
			found["agents"] = true
		}
		if strings.Contains(d, ".claude/skills") {
			found["claude"] = true
		}
		if strings.Contains(d, ".codex/skills") {
			found["codex"] = true
		}
		if strings.Contains(d, ".hawk/skills") {
			found["hawk"] = true
		}
	}
	for _, agent := range []string{"agents", "claude", "codex", "hawk"} {
		if !found[agent] {
			t.Errorf("expected %s skills directory", agent)
		}
	}
	if len(dirs) < 8 {
		t.Errorf("expected at least 8 dirs, got %d", len(dirs))
	}
}
