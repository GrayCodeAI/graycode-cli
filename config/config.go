package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LoadAgentsMD reads AGENTS.md (or AGENTS.md for backward compatibility) from the current directory or parents.
func LoadAgentsMD() string {
	dir, _ := os.Getwd()
	return LoadAgentsMDFrom(dir)
}

const maxAgentsMDSize = 10 * 1024 // 10KB

// agentFiles lists project instruction filenames in priority order.
// AGENTS.md is the canonical name; AGENTS.md is kept for backward compatibility.
var agentFiles = []string{
	"AGENTS.md", ".hawk/AGENTS.md", ".agent/AGENTS.md",
	"AGENTS.md", ".hawk/AGENTS.md", ".agent/AGENTS.md",
}

// LoadAgentsMDFrom reads AGENTS.md (or AGENTS.md fallback) from start or its parents.
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
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				if len(data) > maxAgentsMDSize {
					return string(data[:maxAgentsMDSize]) + "\n\n[WARNING: AGENTS.md truncated to 10KB]"
				}
				return string(data)
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

// LoadAgentDir returns the path to .hawk/ or .agent/ directory, whichever exists.
// .hawk/ takes priority. Returns empty string if neither exists.
func LoadAgentDir() string {
	dir, _ := os.Getwd()
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for _, name := range []string{".hawk", ".agent"} {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
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
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// BuildContext assembles the full context string for the system prompt.
func BuildContext() string {
	return BuildContextWithDirs(nil)
}

// BuildContextWithDirs assembles context including additional user-specified directories.
func BuildContextWithDirs(addDirs []string) string {
	var parts []string
	cwd, _ := os.Getwd()
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	parts = append(parts, "Working directory: "+cwd)
	if git := GitContext(); git != "" {
		parts = append(parts, git)
	}
	if md := LoadAgentsMD(); md != "" {
		parts = append(parts, "Project instructions (AGENTS.md):\n"+md)
	}
	// Cross-agent context files: read instructions from other coding agents.
	crossAgentFiles := []string{
		"CLAUDE.md", "CLAUDE.local.md",
		"GEMINI.md",
		".cursorrules",
		".github/copilot-instructions.md",
		"crush.md", "CRUSH.md",
	}
	for _, name := range crossAgentFiles {
		data, err := os.ReadFile(filepath.Join(cwd, name))
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxAgentsMDSize {
			content = content[:maxAgentsMDSize]
		}
		parts = append(parts, fmt.Sprintf("Cross-agent instructions (%s):\n%s", name, content))
	}
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
		parts = append(parts, "Additional directory: "+dir)
		if md := LoadAgentsMDFrom(dir); md != "" {
			parts = append(parts, "Additional directory instructions ("+dir+"):\n"+md)
		}
	}
	return strings.Join(parts, "\n")
}
