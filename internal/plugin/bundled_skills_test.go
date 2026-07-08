package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// nonSkillDirs are bundled_skills/ subdirectories that intentionally hold
// supporting content rather than a skill (no SKILL.md).
var nonSkillDirs = map[string]bool{
	"references": true,
	"agents":     true,
	"hooks":      true,
}

// TestBundledSkillsMatchDiskCatalog fails when the bundled_skills/ directory
// on disk and the go:embed'ed skill set drift apart: a skill directory whose
// SKILL.md is missing, an embed pattern that silently drops files, or
// frontmatter without a name/description.
func TestBundledSkillsMatchDiskCatalog(t *testing.T) {
	entries, err := os.ReadDir("bundled_skills")
	if err != nil {
		t.Fatalf("read bundled_skills dir: %v", err)
	}

	diskNames := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() || nonSkillDirs[e.Name()] {
			continue
		}
		skillFile := filepath.Join("bundled_skills", e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			t.Errorf("skill directory %q has no readable SKILL.md: %v", e.Name(), err)
			continue
		}
		name, desc, _ := parseSkillFrontmatter(string(data))
		if name == "" {
			name = e.Name()
		}
		if desc == "" {
			t.Errorf("skill %q has no description in SKILL.md frontmatter", e.Name())
		}
		diskNames[name] = true
	}
	if len(diskNames) == 0 {
		t.Fatal("no skill directories found on disk — wrong working directory?")
	}

	embeddedNames := map[string]bool{}
	for _, s := range bundledSkills() {
		if s.Name == "" {
			t.Error("embedded skill with empty name")
			continue
		}
		if s.Description == "" {
			t.Errorf("embedded skill %q has empty description", s.Name)
		}
		embeddedNames[s.Name] = true
	}

	for name := range diskNames {
		if !embeddedNames[name] {
			t.Errorf("skill %q exists on disk but is not embedded — check the go:embed patterns in bundled_skills.go", name)
		}
	}
	for name := range embeddedNames {
		if !diskNames[name] {
			t.Errorf("skill %q is embedded but has no directory on disk", name)
		}
	}
}
