package plugin

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

//go:embed bundled_skills/*/SKILL.md
//go:embed bundled_skills/references/*.md
var bundledSkillsFS embed.FS

// BundledSkill defines a skill that ships with hawk.
type BundledSkill struct {
	Name        string
	Description string
	Category    string
	Content     string
	Files       map[string]string // additional files beyond SKILL.md
}

// bundledSkills returns the set of skills that ship with hawk, loaded
// from the embedded bundled_skills/ directory at compile time.
func bundledSkills() []BundledSkill {
	var skills []BundledSkill

	// Walk the embedded filesystem to find all SKILL.md files
	root := "bundled_skills"
	err := fs.WalkDir(bundledSkillsFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "SKILL.md" {
			return nil
		}

		data, err := bundledSkillsFS.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		name, desc, cat := parseSkillFrontmatter(content)

		// Derive skill name from directory path
		relPath := strings.TrimPrefix(path, root+"/")
		parts := strings.Split(relPath, "/")
		if len(parts) < 2 {
			return nil
		}
		dirName := parts[0]
		if name == "" {
			name = dirName
		}

		skills = append(skills, BundledSkill{
			Name:        name,
			Description: desc,
			Category:    cat,
			Content:     content,
		})
		return nil
	})
	if err != nil {
		return fallbackBundledSkills()
	}

	if len(skills) == 0 {
		return fallbackBundledSkills()
	}

	return skills
}

// parseSkillFrontmatter extracts name, description, and category from
// YAML frontmatter in a SKILL.md file.
func parseSkillFrontmatter(content string) (name, desc, cat string) {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if inFrontmatter {
				break
			}
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			continue
		}
		if strings.HasPrefix(trimmed, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		}
		if strings.HasPrefix(trimmed, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		}
		if strings.HasPrefix(trimmed, "category:") {
			cat = strings.TrimSpace(strings.TrimPrefix(trimmed, "category:"))
		}
	}
	return
}

// fallbackBundledSkills returns a minimal set of skills if the embedded
// filesystem is unavailable (should not happen in normal builds).
func fallbackBundledSkills() []BundledSkill {
	return []BundledSkill{
		{
			Name:        "git-workflow",
			Description: "Best practices for git branching, commits, and PRs",
			Category:    "general",
			Content:     "---\nname: git-workflow\ndescription: Best practices for git branching, commits, and PRs\ncategory: general\n---\n\n# Git Workflow\n\nUse conventional commits, keep subject lines under 72 chars, rebase before PR.",
		},
		{
			Name:        "test-driven",
			Description: "Test-first development workflow",
			Category:    "general",
			Content:     "---\nname: test-driven\ndescription: Test-first development workflow\ncategory: general\n---\n\n# Test-Driven Development\n\nWrite failing test, implement, refactor, repeat.",
		},
		{
			Name:        "code-review",
			Description: "Systematic code review process",
			Category:    "general",
			Content:     "---\nname: code-review\ndescription: Systematic code review process\ncategory: general\n---\n\n# Code Review Process\n\nCheck correctness, readability, performance, security, testing, maintainability.",
		},
	}
}

// BundledSkillsDir returns the directory where bundled skills are extracted.
func BundledSkillsDir() string {
	return storage.BundledSkillsDir()
}

// ExtractBundledSkills extracts bundled skills to the user directory.
// Returns the number of skills extracted.
func ExtractBundledSkills() (int, error) {
	dir := BundledSkillsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create bundled skills directory: %w", err)
	}

	skills := bundledSkills()
	extracted := 0

	for _, skill := range skills {
		skillDir := filepath.Join(dir, skill.Name)

		// Skip if already extracted
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			continue
		}

		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			continue
		}

		if err := os.WriteFile(skillFile, []byte(skill.Content), 0o644); err != nil {
			continue
		}

		// Write additional files
		for name, content := range skill.Files {
			if err := os.WriteFile(filepath.Join(skillDir, name), []byte(content), 0o644); err != nil {
				continue
			}
		}

		extracted++
	}

	// Also extract reference docs
	refDir := filepath.Join(dir, "references")
	if err := os.MkdirAll(refDir, 0o755); err == nil {
		err := fs.WalkDir(bundledSkillsFS, "bundled_skills/references", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := filepath.Base(path)
			dest := filepath.Join(refDir, name)
			if _, statErr := os.Stat(dest); statErr == nil {
				return nil // skip if exists
			}
			data, err := bundledSkillsFS.ReadFile(path)
			if err != nil {
				return nil
			}
			_ = os.WriteFile(dest, data, 0o644)
			return nil
		})
		_ = err
	}

	return extracted, nil
}

// BundledSkillsSummary returns a human-readable list of bundled skills.
func BundledSkillsSummary() string {
	skills := bundledSkills()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Bundled skills (%d):\n\n", len(skills)))
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("  • %s — %s [%s]\n", s.Name, s.Description, s.Category))
	}
	return b.String()
}
