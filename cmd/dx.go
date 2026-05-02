package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/plugin"
)

// startTime records when the process started, used by debugOutput for uptime.
var startTime = time.Now()

// doctorOutput returns a comprehensive system diagnostics string.
func doctorOutput(settings hawkconfig.Settings) string {
	var b strings.Builder
	b.WriteString("=== Hawk Doctor ===\n\n")

	// Go version, OS, arch
	b.WriteString("System:\n")
	b.WriteString(fmt.Sprintf("  Go version:  %s\n", runtime.Version()))
	b.WriteString(fmt.Sprintf("  OS:          %s\n", runtime.GOOS))
	b.WriteString(fmt.Sprintf("  Arch:        %s\n", runtime.GOARCH))

	// Shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "(not set)"
	}
	b.WriteString(fmt.Sprintf("  Shell:       %s\n", shell))

	// Terminal
	termVal := os.Getenv("TERM")
	if termVal == "" {
		termVal = "(not set)"
	}
	colorTerm := os.Getenv("COLORTERM")
	if colorTerm == "" {
		colorTerm = "(not set)"
	}
	b.WriteString(fmt.Sprintf("  TERM:        %s\n", termVal))
	b.WriteString(fmt.Sprintf("  COLORTERM:   %s\n", colorTerm))

	// Hawk version
	v := version
	if v == "" {
		v = "(dev)"
	}
	b.WriteString(fmt.Sprintf("\nHawk:\n"))
	b.WriteString(fmt.Sprintf("  Version:     %s\n", v))
	if buildDate != "" && buildDate != "unknown" {
		b.WriteString(fmt.Sprintf("  Build date:  %s\n", buildDate))
	}

	// Provider + API key status
	effectiveProvider := strings.TrimSpace(settings.Provider)
	if effectiveProvider == "" {
		effectiveProvider = "(not configured)"
	}
	b.WriteString(fmt.Sprintf("\nProvider:\n"))
	b.WriteString(fmt.Sprintf("  Provider:    %s\n", effectiveProvider))
	b.WriteString(fmt.Sprintf("  API key:     %s\n", maskedKeyStatus(settings.Provider)))

	// Model configured
	effectiveModel := strings.TrimSpace(settings.Model)
	if effectiveModel == "" {
		effectiveModel = "(not configured)"
	}
	b.WriteString(fmt.Sprintf("  Model:       %s\n", effectiveModel))

	// Session directory status
	b.WriteString(fmt.Sprintf("\nSession directory:\n"))
	home, _ := os.UserHomeDir()
	sessDir := filepath.Join(home, ".hawk", "sessions")
	if info, err := os.Stat(sessDir); err != nil {
		b.WriteString(fmt.Sprintf("  Status:      missing (%s)\n", sessDir))
	} else if !info.IsDir() {
		b.WriteString(fmt.Sprintf("  Status:      not a directory (%s)\n", sessDir))
	} else {
		writable := "writable"
		testFile := filepath.Join(sessDir, ".dx_write_test")
		if f, err := os.Create(testFile); err != nil {
			writable = "not writable"
		} else {
			f.Close()
			os.Remove(testFile)
		}
		entries, _ := os.ReadDir(sessDir)
		b.WriteString(fmt.Sprintf("  Path:        %s\n", sessDir))
		b.WriteString(fmt.Sprintf("  Status:      exists, %s\n", writable))
		b.WriteString(fmt.Sprintf("  Files:       %d\n", len(entries)))
	}

	// MCP servers configured
	mcpCount := len(settings.MCPServers) + len(mcpServers)
	b.WriteString(fmt.Sprintf("\nMCP servers:     %d\n", mcpCount))

	// Plugins installed
	if manifests, err := plugin.List(); err == nil && len(manifests) > 0 {
		b.WriteString(fmt.Sprintf("Plugins:         %d\n", len(manifests)))
	} else {
		b.WriteString("Plugins:         0\n")
	}

	// AGENTS.md found
	agentsMD := hawkconfig.LoadAgentsMD()
	if agentsMD != "" {
		b.WriteString("AGENTS.md:       found\n")
	} else {
		b.WriteString("AGENTS.md:       not found\n")
	}

	// Git repo status
	b.WriteString(fmt.Sprintf("\nGit:\n"))
	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "" {
		b.WriteString("  Not a git repository\n")
	} else {
		head, _ := gitOutput("rev-parse", "--short", "HEAD")
		b.WriteString(fmt.Sprintf("  Branch:      %s\n", branch))
		if head != "" {
			b.WriteString(fmt.Sprintf("  HEAD:        %s\n", head))
		}
		status, _ := gitOutput("status", "--short")
		if status == "" {
			b.WriteString("  Working tree: clean\n")
		} else {
			lines := strings.Split(status, "\n")
			b.WriteString(fmt.Sprintf("  Modified:    %d files\n", len(lines)))
		}
	}

	// Disk space available
	b.WriteString(fmt.Sprintf("\n%s", diskSpaceInfo()))

	return strings.TrimRight(b.String(), "\n")
}

// maskedKeyStatus returns the API key status for a provider, masking the actual key.
func maskedKeyStatus(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "(no provider set)"
	}
	status := hawkconfig.EnvKeyStatus(provider)
	if status == "set" {
		return "configured (masked)"
	}
	return "missing"
}

// diskSpaceInfo returns available disk space information.
// On darwin/linux, this uses syscall-free approach via os.
func diskSpaceInfo() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "Disk space: unknown\n"
	}
	return fmt.Sprintf("Disk: (check with df -h %s)\n", cwd)
}

// debugOutput returns debugging information about the current session.
func debugOutput(sess *engine.Session, sessionID string) string {
	var b strings.Builder
	b.WriteString("=== Debug Info ===\n\n")

	// Current session ID
	b.WriteString(fmt.Sprintf("Session ID:      %s\n", sessionID))

	// Message count
	b.WriteString(fmt.Sprintf("Message count:   %d\n", sess.MessageCount()))

	// Token estimate (rough: ~4 chars per token across all messages)
	totalChars := 0
	for _, msg := range sess.RawMessages() {
		totalChars += len(msg.Content)
	}
	tokenEstimate := totalChars / 4
	if tokenEstimate == 0 && totalChars > 0 {
		tokenEstimate = 1
	}
	b.WriteString(fmt.Sprintf("Token estimate:  ~%d\n", tokenEstimate))

	// Provider/model in use
	b.WriteString(fmt.Sprintf("Provider:        %s\n", sess.Provider()))
	b.WriteString(fmt.Sprintf("Model:           %s\n", sess.Model()))

	// Compaction status
	if sess.ShouldAutoCompact() {
		b.WriteString("Compaction:      needed (approaching context limit)\n")
	} else {
		b.WriteString("Compaction:      not needed\n")
	}

	// Memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	b.WriteString(fmt.Sprintf("\nMemory:\n"))
	b.WriteString(fmt.Sprintf("  Alloc:         %s\n", formatBytes(memStats.Alloc)))
	b.WriteString(fmt.Sprintf("  TotalAlloc:    %s\n", formatBytes(memStats.TotalAlloc)))
	b.WriteString(fmt.Sprintf("  Sys:           %s\n", formatBytes(memStats.Sys)))
	b.WriteString(fmt.Sprintf("  HeapInuse:     %s\n", formatBytes(memStats.HeapInuse)))
	b.WriteString(fmt.Sprintf("  HeapObjects:   %d\n", memStats.HeapObjects))

	// Goroutine count
	b.WriteString(fmt.Sprintf("\nGoroutines:      %d\n", runtime.NumGoroutine()))

	// Uptime
	uptime := time.Since(startTime).Truncate(time.Second)
	b.WriteString(fmt.Sprintf("Uptime:          %s\n", uptime))

	return strings.TrimRight(b.String(), "\n")
}

// metricsOutput returns resource metrics for the current process.
func metricsOutput(sess *engine.Session) string {
	var b strings.Builder
	b.WriteString("=== Resource Metrics ===\n\n")

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	b.WriteString(fmt.Sprintf("Allocated memory:    %s\n", formatBytes(memStats.Alloc)))
	b.WriteString(fmt.Sprintf("Total allocations:   %s\n", formatBytes(memStats.TotalAlloc)))
	b.WriteString(fmt.Sprintf("GC runs:             %d\n", memStats.NumGC))
	b.WriteString(fmt.Sprintf("Goroutines active:   %d\n", runtime.NumGoroutine()))

	// Open file descriptors (best-effort on darwin/linux)
	fdCount := countOpenFDs()
	if fdCount >= 0 {
		b.WriteString(fmt.Sprintf("Open file descriptors: %d\n", fdCount))
	} else {
		b.WriteString("Open file descriptors: (unavailable)\n")
	}

	// Include engine metrics if available
	if sess != nil && sess.Metrics() != nil {
		engineMetrics := sess.Metrics().Format()
		if engineMetrics != "" && engineMetrics != "No metrics collected." {
			b.WriteString(fmt.Sprintf("\n%s\n", engineMetrics))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// countOpenFDs returns the number of open file descriptors for the current process,
// or -1 if it cannot be determined.
func countOpenFDs() int {
	// On darwin and linux, /dev/fd or /proc/self/fd lists open FDs
	fdDir := "/dev/fd"
	if runtime.GOOS == "linux" {
		fdDir = "/proc/self/fd"
	}
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return -1
	}
	return len(entries)
}

// exportMarkdown exports the session messages to a Markdown file in the current directory.
// Returns the file path and any error.
func exportMarkdown(messages []displayMsg, sessionID string) (string, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Hawk Session: %s\n\n", sessionID))
	b.WriteString(fmt.Sprintf("Exported: %s\n\n", time.Now().Format(time.RFC3339)))
	b.WriteString("---\n\n")

	for _, msg := range messages {
		switch msg.role {
		case "user":
			b.WriteString(fmt.Sprintf("## User\n\n%s\n\n", msg.content))
		case "assistant":
			b.WriteString(fmt.Sprintf("## Assistant\n\n%s\n\n", msg.content))
		case "system":
			b.WriteString(fmt.Sprintf("_System: %s_\n\n", msg.content))
		case "error":
			b.WriteString(fmt.Sprintf("**Error:** %s\n\n", msg.content))
		case "tool_use":
			b.WriteString(fmt.Sprintf("> Tool: %s\n\n", msg.content))
		case "tool_result":
			b.WriteString(fmt.Sprintf("```\n%s\n```\n\n", msg.content))
		case "thinking":
			b.WriteString(fmt.Sprintf("_Thinking: %s_\n\n", msg.content))
		case "welcome":
			// skip welcome banners in export
		default:
			if msg.content != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", msg.content))
			}
		}
	}

	filename := fmt.Sprintf("hawk-session-%s.md", sessionID)
	if err := os.WriteFile(filename, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", filename, err)
	}
	abs, _ := filepath.Abs(filename)
	return abs, nil
}

// exportJSON exports the session messages to a structured JSON file in the current directory.
// Returns the file path and any error.
func exportJSON(messages []displayMsg, sessionID string) (string, error) {
	type exportedMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type exportedSession struct {
		SessionID  string            `json:"session_id"`
		ExportedAt string            `json:"exported_at"`
		Messages   []exportedMessage `json:"messages"`
	}

	var msgs []exportedMessage
	for _, msg := range messages {
		if msg.role == "welcome" {
			continue
		}
		if msg.content == "" {
			continue
		}
		msgs = append(msgs, exportedMessage{
			Role:    msg.role,
			Content: msg.content,
		})
	}

	export := exportedSession{
		SessionID:  sessionID,
		ExportedAt: time.Now().Format(time.RFC3339),
		Messages:   msgs,
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	filename := fmt.Sprintf("hawk-session-%s.json", sessionID)
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", filename, err)
	}
	abs, _ := filepath.Abs(filename)
	return abs, nil
}

// retryLastPrompt finds and returns the last user message for re-sending.
// Returns an empty string if no user messages are found.
func retryLastPrompt(sess *engine.Session) string {
	msgs := sess.RawMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
