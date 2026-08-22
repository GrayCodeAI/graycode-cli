package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSkillFile_CommunityMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "code-review")
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content := "---\nname: code-review\ndescription: Review code safely\nversion: 1.0.0\n---\nUse the review workflow.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ValidateSkillFile(path); len(got) != 0 {
		t.Fatalf("valid skill produced findings: %#v", got)
	}
}

func TestValidateSkillFile_RejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "not-kebab")
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content := "---\nname: Bad_Name\ndescription: \nversion: latest\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ValidateSkillFile(path)
	if len(findings) < 3 {
		t.Fatalf("expected metadata findings, got %#v", findings)
	}
}

func TestValidateSkillFile_RejectsReferenceEscape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "safe-skill")
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content := "---\nname: safe-skill\ndescription: Safe skill\n---\nRead @ref(../secret.md).\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ValidateSkillFile(path)
	if len(findings) != 1 || findings[0].Severity != SeverityCritical {
		t.Fatalf("expected one critical reference finding, got %#v", findings)
	}
}
