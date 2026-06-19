package project

import (
	"fmt"
	"strings"
)

// This file holds the human-readable output formatters for a ProjectAnalysis
// (onboarding doc and concise summary) plus their small formatting helpers.

// GenerateOnboardingDoc produces a human-readable onboarding document from the analysis.
func GenerateOnboardingDoc(analysis *ProjectAnalysis) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Project: %s\n\n", analysis.Name))

	// Architecture section.
	b.WriteString(fmt.Sprintf("## Architecture: %s\n", projAnalyzerTitle(analysis.Architecture)))
	if len(analysis.KeyModules) > 0 {
		moduleNames := make([]string, 0, len(analysis.KeyModules))
		for _, m := range analysis.KeyModules {
			moduleNames = append(moduleNames, m.Name)
		}
		b.WriteString(strings.Join(moduleNames, " -> "))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Key modules section.
	b.WriteString("## Key Modules\n")
	for _, m := range analysis.KeyModules {
		locStr := formatLOC(m.Size)
		purpose := m.Purpose
		if purpose == "" {
			purpose = "Core functionality"
		}
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", m.Name, locStr, purpose))
	}
	b.WriteString("\n")

	// Patterns section.
	if len(analysis.Patterns) > 0 {
		b.WriteString("## Patterns Detected\n")
		for _, p := range analysis.Patterns {
			if p.Confidence >= 0.5 {
				b.WriteString(fmt.Sprintf("- %s (%s)\n", p.Name, p.Description))
			}
		}
		b.WriteString("\n")
	}

	// Conventions section.
	if len(analysis.Conventions) > 0 {
		b.WriteString("## Conventions\n")
		for _, c := range analysis.Conventions {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}

	// Stats section.
	b.WriteString("## Stats\n")
	b.WriteString(fmt.Sprintf("- Language: %s\n", analysis.Language))
	if analysis.Framework != "" {
		b.WriteString(fmt.Sprintf("- Framework: %s\n", analysis.Framework))
	}
	b.WriteString(fmt.Sprintf("- Total LOC: %s\n", formatLOC(analysis.LOC)))
	b.WriteString(fmt.Sprintf("- Dependencies: %d\n", analysis.Dependencies))
	b.WriteString(fmt.Sprintf("- Test Coverage: %s\n", analysis.TestCoverage))
	b.WriteString(fmt.Sprintf("- Complexity: %s\n", analysis.Complexity))

	return b.String()
}

// FormatAnalysis produces a concise summary string from a ProjectAnalysis.
func FormatAnalysis(analysis *ProjectAnalysis) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Project: %s (%s", analysis.Name, analysis.Language))
	if analysis.Framework != "" {
		b.WriteString(fmt.Sprintf(" / %s", analysis.Framework))
	}
	b.WriteString(")\n")

	b.WriteString(fmt.Sprintf("Architecture: %s\n", analysis.Architecture))
	b.WriteString(fmt.Sprintf("LOC: %s | Deps: %d | Tests: %s | Complexity: %s\n",
		formatLOC(analysis.LOC), analysis.Dependencies, analysis.TestCoverage, analysis.Complexity))

	if len(analysis.EntryPoints) > 0 {
		b.WriteString(fmt.Sprintf("Entry Points: %s\n", strings.Join(analysis.EntryPoints, ", ")))
	}

	if len(analysis.KeyModules) > 0 {
		b.WriteString(fmt.Sprintf("Modules: %d key modules\n", len(analysis.KeyModules)))
	}

	if len(analysis.Patterns) > 0 {
		patternNames := make([]string, 0, len(analysis.Patterns))
		for _, p := range analysis.Patterns {
			if p.Confidence >= 0.5 {
				patternNames = append(patternNames, p.Name)
			}
		}
		if len(patternNames) > 0 {
			b.WriteString(fmt.Sprintf("Patterns: %s\n", strings.Join(patternNames, ", ")))
		}
	}

	return b.String()
}

func formatLOC(loc int) string {
	if loc >= 1000 {
		return fmt.Sprintf("%dK LOC", loc/1000)
	}
	return fmt.Sprintf("%d LOC", loc)
}

func projAnalyzerTitle(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
