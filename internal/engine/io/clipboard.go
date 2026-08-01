package io

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ClipboardMonitor watches the system clipboard for changes and fires a callback
// when new content is detected. Inspired by Aider's copypaste feature.
type ClipboardMonitor struct {
	Enabled      bool
	PollInterval time.Duration
	OnPaste      func(content string)

	lastContent string
	done        chan struct{}
	mu          sync.Mutex
}

// NewClipboardMonitor creates a new ClipboardMonitor with sensible defaults.
func NewClipboardMonitor() *ClipboardMonitor {
	return &ClipboardMonitor{
		Enabled:      true,
		PollInterval: 1 * time.Second,
		done:         make(chan struct{}),
	}
}

// Start begins polling the clipboard at PollInterval. When content changes and
// passes validation (>20 chars, <50000 chars), the OnPaste callback is fired.
// The monitor runs until Stop() is called or the context is cancelled.
func (cm *ClipboardMonitor) Start(ctx context.Context) error {
	if !cm.Enabled {
		return nil
	}

	// Seed with current clipboard content so we don't fire on existing content.
	initial, err := ReadClipboard()
	if err == nil {
		cm.mu.Lock()
		cm.lastContent = initial
		cm.mu.Unlock()
	}

	go func() {
		ticker := time.NewTicker(cm.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-cm.done:
				return
			case <-ticker.C:
				cm.poll()
			}
		}
	}()

	return nil
}

// Stop halts the clipboard polling loop.
func (cm *ClipboardMonitor) Stop() {
	select {
	case <-cm.done:
		// already closed
	default:
		close(cm.done)
	}
}

func (cm *ClipboardMonitor) poll() {
	content, err := ReadClipboard()
	if err != nil {
		return
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if content == cm.lastContent {
		return
	}

	cm.lastContent = content

	// Only fire for substantial content.
	if len(content) <= 20 || len(content) >= 50000 {
		return
	}

	if cm.OnPaste != nil {
		cm.OnPaste(content)
	}
}

// ReadClipboard reads the current system clipboard content.
// It uses platform-specific commands:
//   - macOS: pbpaste
//   - Linux: xclip -selection clipboard -o (falls back to xsel)
//   - Windows: powershell Get-Clipboard
func ReadClipboard() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "pbpaste")
	case "linux":
		// Try xclip first, fall back to xsel.
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(context.Background(), "xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(context.Background(), "xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("clipboard: no clipboard tool found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.CommandContext(context.Background(), "powershell", "-command", "Get-Clipboard")
	default:
		return "", fmt.Errorf("clipboard: unsupported platform %s", runtime.GOOS)
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("clipboard read: %w", err)
	}
	return string(out), nil
}

// WriteClipboard writes the given content to the system clipboard.
// It uses platform-specific commands:
//   - macOS: pipe to pbcopy
//   - Linux: pipe to xclip -selection clipboard (falls back to xsel)
//   - Windows: pipe to powershell Set-Clipboard
func WriteClipboard(content string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(context.Background(), "xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(context.Background(), "xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("clipboard: no clipboard tool found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.CommandContext(context.Background(), "powershell", "-command", "Set-Clipboard", "-Value", content) // #nosec G204 -- fixed PowerShell command; clipboard content is one isolated argument
		return cmd.Run()
	default:
		return fmt.Errorf("clipboard: unsupported platform %s", runtime.GOOS)
	}

	cmd.Stdin = bytes.NewBufferString(content)
	return cmd.Run()
}

// DetectContentType classifies clipboard content into a category.
// Returns one of: "code", "diff", "url", "path", "error", "text".
func DetectContentType(content string) string {
	trimmed := strings.TrimSpace(content)

	// Check for diff first (most specific).
	if strings.HasPrefix(trimmed, "diff --git") || isDiffContent(trimmed) {
		return "diff"
	}

	// Check for URL.
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		// Single-line URL check.
		if !strings.Contains(trimmed, "\n") || len(strings.Split(trimmed, "\n")) <= 2 {
			return "url"
		}
	}

	// Check for error/stack trace.
	if isErrorContent(trimmed) {
		return "error"
	}

	// Check for file path.
	if isPathContent(trimmed) {
		return "path"
	}

	// Check for code.
	if isCodeContent(trimmed) {
		return "code"
	}

	return "text"
}

// FormatForContext wraps content appropriately for injection into agent context
// based on its detected content type.
func FormatForContext(content string, contentType string) string {
	switch contentType {
	case "code":
		lang := DetectLanguage(content)
		return fmt.Sprintf("```%s\n%s\n```", lang, content)
	case "diff":
		return fmt.Sprintf("```diff\n%s\n```", content)
	case "error":
		return fmt.Sprintf("Error from clipboard:\n```\n%s\n```", content)
	case "url":
		return fmt.Sprintf("URL from clipboard: %s", strings.TrimSpace(content))
	case "path":
		return fmt.Sprintf("Path from clipboard: %s", strings.TrimSpace(content))
	default:
		return content
	}
}

// DetectLanguage performs heuristic language detection from code content.
// Returns the detected language name or "" if unknown.
func DetectLanguage(code string) string {
	lines := strings.Split(code, "\n")

	var (
		hasFunc      bool
		hasPackage   bool
		hasDef       bool
		hasColonEnd  bool
		hasConst     bool
		hasArrow     bool
		hasImportBr  bool
		hasFn        bool
		hasLet       bool
		hasThinArrow bool
		hasClass     bool
		hasPublic    bool
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "func ") {
			hasFunc = true
		}
		if strings.HasPrefix(trimmed, "package ") {
			hasPackage = true
		}
		if strings.HasPrefix(trimmed, "def ") {
			hasDef = true
		}
		if strings.HasSuffix(trimmed, ":") {
			hasColonEnd = true
		}
		if strings.Contains(trimmed, "const ") {
			hasConst = true
		}
		if strings.Contains(trimmed, "=>") {
			hasArrow = true
		}
		if strings.Contains(trimmed, "import {") || strings.Contains(trimmed, "import type {") {
			hasImportBr = true
		}
		if strings.HasPrefix(trimmed, "fn ") || strings.Contains(trimmed, " fn ") {
			hasFn = true
		}
		if strings.HasPrefix(trimmed, "let ") || strings.Contains(trimmed, " let ") {
			hasLet = true
		}
		if strings.Contains(trimmed, "->") {
			hasThinArrow = true
		}
		if strings.HasPrefix(trimmed, "class ") || strings.Contains(trimmed, " class ") {
			hasClass = true
		}
		if strings.HasPrefix(trimmed, "public ") || strings.Contains(trimmed, " public ") {
			hasPublic = true
		}
	}

	// Go: func + package
	if hasFunc && hasPackage {
		return "go"
	}

	// Python: def + colon at end of line
	if hasDef && hasColonEnd {
		return "python"
	}

	// TypeScript: const + => or import {
	if hasConst && hasArrow || hasImportBr {
		return "typescript"
	}

	// Rust: fn + let + ->
	if hasFn && hasLet && hasThinArrow {
		return "rust"
	}

	// Java: class + public
	if hasClass && hasPublic {
		return "java"
	}

	return ""
}

// SummarizeClipboard truncates large clipboard content for display,
// showing the first and last few lines with an omission indicator.
func SummarizeClipboard(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}

	lines := strings.Split(content, "\n")
	if len(lines) <= 6 {
		// Few lines but long content: simple truncation.
		return content[:maxChars] + "..."
	}

	// Show first 3 and last 3 lines.
	head := strings.Join(lines[:3], "\n")
	tail := strings.Join(lines[len(lines)-3:], "\n")
	omitted := len(lines) - 6
	return fmt.Sprintf("%s\n... %d lines omitted ...\n%s", head, omitted, tail)
}

// --- internal helpers ---

var diffLinePattern = regexp.MustCompile(`^[+-]{1}[^+-]`)

func isDiffContent(content string) bool {
	lines := strings.Split(content, "\n")
	diffLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			diffLines += 2
		} else if diffLinePattern.MatchString(line) {
			diffLines++
		} else if strings.HasPrefix(line, "@@ ") {
			diffLines += 2
		}
	}
	// If more than 30% of lines look like diff markers, treat as diff.
	return len(lines) > 3 && diffLines*100/len(lines) > 30
}

func isErrorContent(content string) bool {
	indicators := []string{
		"Error:", "Exception:", "Traceback (most recent call last)",
		"panic:", "FATAL", "at Object.", "at Module.",
		"goroutine ", "stack trace:",
	}
	for _, ind := range indicators {
		if strings.Contains(content, ind) {
			return true
		}
	}
	// Stack trace pattern: repeated "at " lines or file:line patterns.
	lines := strings.Split(content, "\n")
	atLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "at ") {
			atLines++
		}
	}
	return atLines >= 3
}

func isPathContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	// Single line only.
	if strings.Contains(trimmed, "\n") {
		return false
	}
	// Unix absolute path or relative path with extension.
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "~/") {
		return true
	}
	// Windows path.
	if len(trimmed) >= 3 && trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/') {
		return true
	}
	// Has a file extension and path separators.
	if (strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\")) && strings.Contains(trimmed, ".") {
		parts := strings.Split(trimmed, ".")
		ext := parts[len(parts)-1]
		if len(ext) >= 1 && len(ext) <= 5 && !strings.Contains(ext, " ") {
			return true
		}
	}
	return false
}

func isCodeContent(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return false
	}

	indentedLines := 0
	bracketLines := 0
	codeKeywords := 0

	keywords := []string{
		"func ", "def ", "class ", "import ", "return ",
		"if ", "for ", "while ", "var ", "let ", "const ",
		"package ", "fn ", "pub ", "struct ", "enum ",
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
			indentedLines++
		}
		trimmed := strings.TrimSpace(line)
		if strings.ContainsAny(trimmed, "{}();") {
			bracketLines++
		}
		for _, kw := range keywords {
			if strings.Contains(trimmed, kw) {
				codeKeywords++
				break
			}
		}
	}

	total := len(lines)
	// Code typically has significant indentation and/or brackets.
	hasIndentation := indentedLines*100/total > 30
	hasBrackets := bracketLines*100/total > 15
	hasKeywords := codeKeywords >= 2

	return (hasIndentation && hasBrackets) || (hasIndentation && hasKeywords) || (hasBrackets && hasKeywords)
}
