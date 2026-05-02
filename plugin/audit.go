package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// AuditSeverity indicates how dangerous a finding is.
type AuditSeverity string

const (
	SeverityCritical AuditSeverity = "CRITICAL"
	SeverityWarning  AuditSeverity = "WARNING"
	SeverityInfo     AuditSeverity = "INFO"
)

// AuditFinding is a single security issue found in a skill file.
type AuditFinding struct {
	File     string
	Line     int
	Column   int
	Severity AuditSeverity
	Category string
	Message  string
	Char     rune
}

// AuditResult is the result of scanning one or more skill files.
type AuditResult struct {
	Findings []AuditFinding
	Files    int
}

// dangerousRanges defines Unicode ranges that are dangerous in skill files.
var dangerousRanges = []struct {
	lo, hi   rune
	category string
	severity AuditSeverity
	desc     string
}{
	// BiDi override characters — can reverse displayed text direction.
	{0x202A, 0x202E, "bidi-override", SeverityCritical, "BiDi override character"},
	{0x2066, 0x2069, "bidi-isolate", SeverityCritical, "BiDi isolate character"},
	// Zero-width characters — invisible, can hide content.
	{0x200B, 0x200B, "zero-width", SeverityWarning, "zero-width space"},
	{0x200C, 0x200C, "zero-width", SeverityWarning, "zero-width non-joiner"},
	{0x200D, 0x200D, "zero-width", SeverityWarning, "zero-width joiner"},
	{0xFEFF, 0xFEFF, "zero-width", SeverityWarning, "zero-width no-break space (BOM)"},
	// Unicode tag characters — can encode hidden instructions.
	{0xE0001, 0xE007F, "unicode-tag", SeverityCritical, "Unicode tag character"},
	// Variation selectors — can alter glyph rendering.
	{0xFE00, 0xFE0F, "variation-selector", SeverityWarning, "variation selector"},
	// Homoglyph-prone: Cyrillic characters that look like Latin.
	{0x0400, 0x04FF, "homoglyph", SeverityInfo, "Cyrillic character (potential homoglyph)"},
}

// AuditSkillFile scans a single file for dangerous Unicode characters.
func AuditSkillFile(path string) ([]AuditFinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return auditContent(path, string(data)), nil
}

func auditContent(path, content string) []AuditFinding {
	var findings []AuditFinding
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		for col, r := range line {
			if r < 128 {
				continue // Fast path: ASCII is safe.
			}
			for _, dr := range dangerousRanges {
				if r >= dr.lo && r <= dr.hi {
					findings = append(findings, AuditFinding{
						File:     path,
						Line:     lineNum + 1,
						Column:   col + 1,
						Severity: dr.severity,
						Category: dr.category,
						Message:  fmt.Sprintf("%s (U+%04X)", dr.desc, r),
						Char:     r,
					})
					break
				}
			}
		}
	}
	return findings
}

// AuditSkillDir scans all SKILL.md files in a directory tree.
func AuditSkillDir(dir string) AuditResult {
	var result AuditResult
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if name != "skill.md" && !strings.HasSuffix(name, ".md") {
			return nil
		}
		result.Files++
		findings, err := AuditSkillFile(path)
		if err != nil {
			return nil
		}
		result.Findings = append(result.Findings, findings...)
		return nil
	})
	return result
}

// AuditAllSkills scans all skill directories.
func AuditAllSkills() AuditResult {
	dirs := DefaultSkillDirs()
	var combined AuditResult
	for _, dir := range dirs {
		r := AuditSkillDir(dir)
		combined.Files += r.Files
		combined.Findings = append(combined.Findings, r.Findings...)
	}
	return combined
}

// FormatAuditResult formats audit findings for display.
func FormatAuditResult(r AuditResult) string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf("Scanned %d file(s). No security issues found. ✓", r.Files)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Scanned %d file(s). Found %d issue(s):\n\n", r.Files, len(r.Findings))

	critical, warning, info := 0, 0, 0
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityCritical:
			critical++
		case SeverityWarning:
			warning++
		case SeverityInfo:
			info++
		}
		fmt.Fprintf(&b, "  [%s] %s:%d:%d — %s\n", f.Severity, f.File, f.Line, f.Column, f.Message)
	}

	b.WriteString("\n")
	if critical > 0 {
		fmt.Fprintf(&b, "⚠ %d CRITICAL finding(s) — these skills may contain hidden malicious content.\n", critical)
	}
	if warning > 0 {
		fmt.Fprintf(&b, "  %d WARNING(s) — invisible characters that may hide content.\n", warning)
	}
	if info > 0 {
		fmt.Fprintf(&b, "  %d INFO — potential homoglyphs (may be legitimate non-Latin text).\n", info)
	}
	return b.String()
}

// StripDangerousChars removes dangerous Unicode characters from content.
func StripDangerousChars(content string) string {
	var b strings.Builder
	b.Grow(len(content))
	for _, r := range content {
		if isDangerous(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isDangerous(r rune) bool {
	if r < 128 {
		return false
	}
	// Only strip critical and warning-level characters, not info (Cyrillic).
	for _, dr := range dangerousRanges {
		if r >= dr.lo && r <= dr.hi && dr.severity != SeverityInfo {
			return true
		}
	}
	// Also flag control characters that aren't standard whitespace.
	if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
		return true
	}
	return false
}
