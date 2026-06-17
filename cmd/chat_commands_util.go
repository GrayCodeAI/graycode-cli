package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/taste"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/system/staleness"
)

func gitOutput(args ...string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func branchSummary() string {
	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "" {
		return "No git repository detected."
	}
	head, _ := gitOutput("rev-parse", "--short", "HEAD")
	upstream, _ := gitOutput("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	status, _ := gitOutput("status", "--short", "--branch")
	var b strings.Builder
	b.WriteString("Branch: " + branch)
	if head != "" {
		b.WriteString(" @ " + head)
	}
	if upstream != "" {
		b.WriteString("\nUpstream: " + upstream)
	}
	if status != "" {
		b.WriteString("\n\n" + status)
	}
	return b.String()
}

func filesSummary() string {
	status, err := gitOutput("status", "--short")
	if err != nil {
		return "No git repository detected."
	}
	if strings.TrimSpace(status) == "" {
		return "No modified files."
	}
	return "Modified files:\n" + status
}

func additionalDirContext(dir string) (string, string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", "", fmt.Errorf("directory path is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%s is not a directory", abs)
	}
	var b strings.Builder
	b.WriteString("Additional directory: " + abs)
	if md := hawkconfig.LoadAgentsMDFrom(abs); md != "" {
		b.WriteString("\nAdditional directory instructions (" + abs + "):\n" + md)
	}
	return abs, b.String(), nil
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (m *chatModel) mcpSummary() string {
	var b strings.Builder
	configured := len(m.settings.MCPServers) + len(mcpServers)
	if configured == 0 {
		b.WriteString("No MCP servers configured.")
	} else {
		b.WriteString(fmt.Sprintf("MCP servers configured: %d\n", configured))
		for _, cfg := range m.settings.MCPServers {
			name := cfg.Name
			if name == "" {
				name = cfg.Command
			}
			b.WriteString(fmt.Sprintf("  %s: %s %s\n", name, cfg.Command, strings.Join(cfg.Args, " ")))
		}
		for _, cmd := range mcpServers {
			b.WriteString("  cli: " + cmd + "\n")
		}
	}
	if m.registry != nil {
		var toolNames []string
		for _, t := range m.registry.EyrieTools() {
			if strings.HasPrefix(t.Name, "mcp__") {
				toolNames = append(toolNames, t.Name)
			}
		}
		if len(toolNames) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString("Connected MCP tools:\n  " + strings.Join(toolNames, "\n  "))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func sessionStats(sess *engine.Session, id string) string {
	return fmt.Sprintf("Session: %s\nMessages: %d\nModel: %s/%s\n%s",
		id, sess.MessageCount(), sess.Provider(), sess.Model(), sess.Cost.Summary())
}

func hooksSummary() string {
	return "Hooks: pre_query, post_query, pre_tool, post_tool, session_start, session_end, permission_ask, error\nConfigure in .hawk/settings.json or ~/.hawk/settings.json"
}

func pluginsSummary(rt *plugin.Runtime) string {
	if rt == nil {
		return "No plugins loaded."
	}
	plugins := rt.ListPlugins()
	if len(plugins) == 0 {
		return "No plugins installed."
	}
	var b strings.Builder
	b.WriteString("Installed plugins:\n")
	for _, p := range plugins {
		b.WriteString(fmt.Sprintf("  %s (%s)\n", p.Name, p.Version))
	}
	return b.String()
}

// tasteStoreForSession returns a taste store using the default location.
func tasteStoreForSession() (*taste.Store, error) {
	return taste.NewStore("")
}

// stalenessFormatReport formats stale rules for display.
func stalenessFormatReport(rules []staleness.StaleRule) string {
	return staleness.FormatReport(rules)
}
