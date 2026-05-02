package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LearnContext holds project analysis data for the LLM advisor.
type LearnContext struct {
	Signals    []ProjectSignal
	Installed  []SmartSkill
	Registry   []SkillEntry
	SourceInfo string // populated by /learn deep
}

// BuildLearnPrompt creates the LLM prompt for skill recommendation.
func BuildLearnPrompt(ctx LearnContext) string {
	var b strings.Builder

	b.WriteString("You are a skill advisor for the Hawk AI coding agent. ")
	b.WriteString("Analyze this project and recommend which community skills to install.\n\n")

	// Project signals.
	b.WriteString("## Project Analysis\n")
	if len(ctx.Signals) == 0 {
		b.WriteString("No project markers detected.\n")
	} else {
		for _, s := range ctx.Signals {
			fmt.Fprintf(&b, "- %s: %s\n", s.Category, s.Name)
		}
	}

	// Source info from /learn deep.
	if ctx.SourceInfo != "" {
		b.WriteString("\n## Source File Analysis\n")
		b.WriteString(ctx.SourceInfo)
		b.WriteString("\n")
	}

	// Installed skills.
	b.WriteString("\n## Currently Installed Skills\n")
	if len(ctx.Installed) == 0 {
		b.WriteString("None.\n")
	} else {
		for _, s := range ctx.Installed {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}
	}

	// Available registry skills.
	b.WriteString("\n## Available Community Skills\n")
	if len(ctx.Registry) == 0 {
		b.WriteString("Registry unavailable.\n")
	} else {
		for _, s := range ctx.Registry {
			fmt.Fprintf(&b, "- **%s** [%s] (%d installs): %s\n", s.Name, s.Category, s.Installs, s.Description)
		}
	}

	b.WriteString("\n## Instructions\n")
	b.WriteString("1. Score each community skill 0-100 for relevance to this project.\n")
	b.WriteString("2. Only recommend skills scoring above 60.\n")
	b.WriteString("3. Flag any installed skills that are redundant or outdated.\n")
	b.WriteString("4. If no community skill scores above 60, suggest what a custom skill should cover.\n")
	b.WriteString("5. Format your response as:\n\n")
	b.WriteString("### Recommended Skills\n")
	b.WriteString("- skill-name (score) — why it's relevant\n\n")
	b.WriteString("### Redundant Installed Skills\n")
	b.WriteString("- skill-name — why it's redundant\n\n")
	b.WriteString("### Custom Skill Suggestion\n")
	b.WriteString("If needed, describe what a custom skill should cover.\n")

	return b.String()
}

// GatherLearnContext collects project info for the advisor.
func GatherLearnContext(dir string) LearnContext {
	signals := AnalyzeProject(dir)
	installed := LoadSmartSkills(DefaultSkillDirs())

	var registry []SkillEntry
	rc := NewRegistryClient()
	if idx, err := rc.FetchIndex(); err == nil {
		registry = idx.Skills
	}

	return LearnContext{
		Signals:   signals,
		Installed: installed,
		Registry:  registry,
	}
}

// GatherDeepSourceInfo reads key source files to provide richer context.
func GatherDeepSourceInfo(dir string) string {
	var b strings.Builder
	maxFileSize := 2000 // chars per file
	maxFiles := 10
	read := 0

	// Priority files to read.
	priority := []string{
		"AGENTS.md", "README.md",
		"go.mod", "package.json", "Cargo.toml", "pyproject.toml", "requirements.txt",
		"Makefile", "Dockerfile", "docker-compose.yml",
		".github/workflows/ci.yml", ".github/workflows/ci.yaml",
	}

	for _, name := range priority {
		if read >= maxFiles {
			break
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxFileSize {
			content = content[:maxFileSize] + "\n... (truncated)"
		}
		fmt.Fprintf(&b, "### %s\n```\n%s\n```\n\n", name, content)
		read++
	}

	// Scan for main entry points.
	entryPoints := []string{
		"main.go", "cmd/root.go", "src/index.ts", "src/main.ts",
		"app.py", "main.py", "src/main.rs", "src/lib.rs",
	}
	for _, name := range entryPoints {
		if read >= maxFiles {
			break
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxFileSize {
			content = content[:maxFileSize] + "\n... (truncated)"
		}
		fmt.Fprintf(&b, "### %s\n```\n%s\n```\n\n", name, content)
		read++
	}

	if b.Len() == 0 {
		return "No key source files found."
	}
	return b.String()
}

// BuildLearnUpdatePrompt creates a prompt to re-analyze installed skills for staleness.
func BuildLearnUpdatePrompt(ctx LearnContext) string {
	var b strings.Builder
	b.WriteString("You are a skill maintenance advisor for the Hawk AI coding agent. ")
	b.WriteString("Re-analyze the installed skills and flag any that are outdated, redundant, or mismatched.\n\n")

	b.WriteString("## Project Analysis\n")
	for _, s := range ctx.Signals {
		fmt.Fprintf(&b, "- %s: %s\n", s.Category, s.Name)
	}

	if ctx.SourceInfo != "" {
		b.WriteString("\n## Source File Analysis\n")
		b.WriteString(ctx.SourceInfo)
		b.WriteString("\n")
	}

	b.WriteString("\n## Installed Skills (review each)\n")
	for _, s := range ctx.Installed {
		fmt.Fprintf(&b, "- **%s**", s.Name)
		if s.Version != "" {
			fmt.Fprintf(&b, " v%s", s.Version)
		}
		if s.Description != "" {
			fmt.Fprintf(&b, ": %s", s.Description)
		}
		if s.Source.Repo != "" {
			fmt.Fprintf(&b, " (from %s)", s.Source.Repo)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## Instructions\n")
	b.WriteString("For each installed skill, assess:\n")
	b.WriteString("1. **Relevance** — does it match the current project?\n")
	b.WriteString("2. **Freshness** — is the version/content likely outdated?\n")
	b.WriteString("3. **Redundancy** — does it overlap with another installed skill?\n")
	b.WriteString("4. **Gaps** — what skills are missing that the project needs?\n\n")
	b.WriteString("Format your response as:\n\n")
	b.WriteString("### Keep\n- skill-name — why it's still useful\n\n")
	b.WriteString("### Update\n- skill-name — what changed, run `/skills update name`\n\n")
	b.WriteString("### Remove\n- skill-name — why it's no longer needed, run `/skills remove name`\n\n")
	b.WriteString("### Install\n- skill-name — why it's needed, run `/skills install repo name`\n")

	return b.String()
}

// FormatLearnSummary creates a display header for the /learn command.
func FormatLearnSummary(ctx LearnContext, deep bool) string {
	var b strings.Builder
	mode := "/learn"
	if deep {
		mode = "/learn deep"
	}
	fmt.Fprintf(&b, "Running %s advisor...\n\n", mode)

	if len(ctx.Signals) > 0 {
		names := make([]string, len(ctx.Signals))
		for i, s := range ctx.Signals {
			names[i] = s.Name
		}
		fmt.Fprintf(&b, "Detected: %s\n", strings.Join(names, ", "))
	}
	fmt.Fprintf(&b, "Installed skills: %d\n", len(ctx.Installed))
	fmt.Fprintf(&b, "Registry skills: %d\n", len(ctx.Registry))
	if deep && ctx.SourceInfo != "" {
		b.WriteString("Source analysis: included\n")
	}
	return b.String()
}
