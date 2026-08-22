package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
)

// LoadAgentsMD reads AGENTS.md from the current directory or parents.
func LoadAgentsMD() string {
	dir, _ := os.Getwd()
	return LoadAgentsMDFrom(dir)
}

const maxAgentsMDSize = 10 * 1024 // 10KB

// agentFiles lists project instruction filenames in priority order.
var agentFiles = []string{
	"AGENTS.md",
}

// LoadAgentsMDFrom reads AGENTS.md from start or its parents.
func LoadAgentsMDFrom(start string) string {
	dir := start
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for {
		for _, name := range agentFiles {
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path) // #nosec G304 -- dir is the working directory or an ancestor of it; name is a fixed constant
			if err == nil {
				content := string(data)
				if len(data) > maxAgentsMDSize {
					content = content[:maxAgentsMDSize] + "\n\n[WARNING: AGENTS.md truncated to 10KB]"
				}
				// Expand `@path` references (bounded by the git root and strict
				// size/depth budgets) so AGENTS.md can pull in in-repo files.
				return expandContextReferences(content, dir, gitRoot(dir))
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// LoadAgentDir returns the path to .agent/ if it exists.
func LoadAgentDir() string {
	dir, _ := os.Getwd()
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	p := filepath.Join(dir, ".agent")
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

// GitContext returns git info for the system prompt.
func GitContext() string {
	var b strings.Builder
	if branch, err := gitCmd("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		b.WriteString("Git branch: " + branch + "\n")
	}
	if user, err := gitCmd("config", "user.name"); err == nil {
		b.WriteString("Git user: " + user + "\n")
	}
	if defaultBranch, err := gitCmd("symbolic-ref", "refs/remotes/origin/HEAD", "--short"); err == nil {
		b.WriteString("Default branch: " + defaultBranch + "\n")
	}
	if log, err := gitCmd("log", "--oneline", "-5"); err == nil && log != "" {
		b.WriteString("Recent commits:\n" + log + "\n")
	}
	if status, err := gitCmd("status", "--porcelain"); err == nil && status != "" {
		lines := strings.Split(status, "\n")
		if len(lines) > 10 {
			lines = append(lines[:10], fmt.Sprintf("... and %d more", len(lines)-10))
		}
		b.WriteString("Modified files:\n" + strings.Join(lines, "\n") + "\n")
	}
	return b.String()
}

func gitCmd(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", args...).Output() // #nosec G204 -- fixed git executable
	return strings.TrimSpace(string(out)), err
}

// loadYaadMemories queries yaad for project-relevant memories and injects them
// into the system prompt. Uses the yaad bridge with project-specific search.
func loadYaadMemories(projectDir string) string {
	if projectDir == "" {
		return ""
	}
	// Use the yaad bridge to search for project context.
	// The bridge handles graceful degradation if yaad is unavailable.
	return memory.LoadYaadContext(projectDir)
}

// BuildContext assembles the full context string for the system prompt.
func BuildContext() string {
	return BuildContextWithDirs(nil)
}

// BuildStartupContextWithDirs assembles only the cheap context needed before
// the first UI paint. Expensive repository scans are deferred.
func BuildStartupContextWithDirs(addDirs []string) string {
	cwd, extras := normalizeContextDirs(addDirs)
	parts := []string{"Working directory: " + cwd}
	for _, dir := range extras {
		parts = append(parts, "Additional directory: "+dir)
	}
	return strings.Join(parts, "\n")
}

// BuildDeferredContextWithDirs assembles the heavier repository-specific
// context that can safely be appended after the initial UI render.
func BuildDeferredContextWithDirs(addDirs []string) string {
	cwd, extras := normalizeContextDirs(addDirs)
	var parts []string
	if git := GitContext(); git != "" {
		parts = append(parts, git)
	}
	if md := LoadAgentsMDFrom(cwd); md != "" {
		parts = append(parts, "Project instructions (AGENTS.md):\n"+md)
	}
	parts = append(parts, loadCrossAgentInstructions(cwd)...)
	for _, dir := range extras {
		if md := LoadAgentsMDFrom(dir); md != "" {
			parts = append(parts, "Additional directory instructions ("+dir+"):\n"+md)
		}
	}
	if yaad := loadYaadMemories(cwd); yaad != "" {
		parts = append(parts, yaad)
	}
	return strings.Join(parts, "\n")
}

// BuildContextWithDirs assembles context including additional user-specified directories.
func BuildContextWithDirs(addDirs []string) string {
	startupCtx := BuildStartupContextWithDirs(addDirs)
	deferredCtx := BuildDeferredContextWithDirs(addDirs)
	switch {
	case startupCtx == "":
		return deferredCtx
	case deferredCtx == "":
		return startupCtx
	default:
		return startupCtx + "\n" + deferredCtx
	}
}

func normalizeContextDirs(addDirs []string) (string, []string) {
	cwd, _ := os.Getwd()
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	var extras []string
	for _, dir := range addDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		if dir == cwd {
			continue
		}
		extras = append(extras, dir)
	}
	return cwd, extras
}

func loadCrossAgentInstructions(cwd string) []string {
	crossAgentFiles := []string{
		"CLAUDE.md", "CLAUDE.local.md",
		"GEMINI.md",
		".cursorrules",
		".github/copilot-instructions.md",
		"crush.md", "CRUSH.md",
	}
	var parts []string
	for _, name := range crossAgentFiles {
		data, err := os.ReadFile(filepath.Join(cwd, name)) // #nosec G304 -- cwd is the process working directory; name is drawn from a fixed constant list
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxAgentsMDSize {
			content = content[:maxAgentsMDSize]
		}
		parts = append(parts, fmt.Sprintf("Cross-agent instructions (%s):\n%s", name, content))
	}
	return parts
}
