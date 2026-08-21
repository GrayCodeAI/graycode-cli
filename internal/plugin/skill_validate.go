package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillValidationFinding is a structural or security-adjacent skill metadata
// problem found before a skill is activated.
type SkillValidationFinding struct {
	Path     string
	Severity AuditSeverity
	Message  string
}

var (
	skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverPattern    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
)

// ValidateSkillFile checks the portable metadata and local references of a
// SKILL.md. It complements AuditSkillFile, which scans content for hidden
// Unicode threats.
func ValidateSkillFile(path string) []SkillValidationFinding {
	data, err := os.ReadFile(path) // #nosec G304 -- caller supplies a local skill path for explicit validation
	if err != nil {
		return []SkillValidationFinding{{Path: path, Severity: SeverityCritical, Message: "read skill: " + err.Error()}}
	}
	skill := parseSmartSkill(string(data))
	findings := make([]SkillValidationFinding, 0)
	dirName := filepath.Base(filepath.Dir(path))
	if skill.Name == "" {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityCritical, Message: "missing required name"})
	} else if !skillNamePattern.MatchString(skill.Name) {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityCritical, Message: fmt.Sprintf("name %q must be lowercase kebab-case", skill.Name)})
	} else if dirName != "" && dirName != "." && skill.Name != dirName {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityWarning, Message: fmt.Sprintf("name %q does not match directory %q", skill.Name, dirName)})
	}
	if strings.TrimSpace(skill.Description) == "" {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityCritical, Message: "missing required description"})
	} else if len([]rune(skill.Description)) > 280 {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityWarning, Message: "description exceeds 280 characters"})
	}
	if skill.Version != "" && !semverPattern.MatchString(skill.Version) {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityWarning, Message: fmt.Sprintf("version %q is not semantic version format", skill.Version)})
	}
	if len(skill.Content) > 500*1024 {
		findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityWarning, Message: "SKILL.md exceeds 500 KiB"})
	}
	for _, ref := range skill.Refs {
		if filepath.Base(ref) != ref || strings.Contains(ref, "..") {
			findings = append(findings, SkillValidationFinding{Path: path, Severity: SeverityCritical, Message: fmt.Sprintf("reference %q escapes references directory", ref)})
		}
	}
	return findings
}

// ValidateSkillDir validates every Markdown skill file below dir.
func ValidateSkillDir(dir string) []SkillValidationFinding {
	var findings []SkillValidationFinding
	entries, err := os.ReadDir(dir)
	if err != nil {
		return findings
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		findings = append(findings, ValidateSkillFile(path)...)
	}
	return findings
}
