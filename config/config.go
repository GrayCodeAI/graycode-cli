package config

import (
	"fmt"
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
