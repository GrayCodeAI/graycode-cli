package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LoadHawkMD reads HAWK.md from the current directory or parents.
func LoadHawkMD() string {
	dir, _ := os.Getwd()
	for {
		for _, name := range []string{"HAWK.md", ".hawk/HAWK.md"} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
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

// GitContext returns a brief git status string for the system prompt.
func GitContext() string {
	var b strings.Builder
	if branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		b.WriteString("Git branch: ")
		b.WriteString(strings.TrimSpace(string(branch)))
		b.WriteString("\n")
	}
	if status, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		s := strings.TrimSpace(string(status))
		if s != "" {
			lines := strings.Split(s, "\n")
			if len(lines) > 10 {
				lines = append(lines[:10], "...")
			}
			b.WriteString("Modified files:\n")
			b.WriteString(strings.Join(lines, "\n"))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// BuildContext assembles the full context string for the system prompt.
func BuildContext() string {
	var parts []string
	cwd, _ := os.Getwd()
	parts = append(parts, "Working directory: "+cwd)
	if git := GitContext(); git != "" {
		parts = append(parts, git)
	}
	if md := LoadHawkMD(); md != "" {
		parts = append(parts, "Project instructions (HAWK.md):\n"+md)
	}
	return strings.Join(parts, "\n")
}
