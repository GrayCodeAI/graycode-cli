package io

import (
	"context"
	"os/exec"
	"strings"
)

// ClipboardBridge enables paste-from-browser workflows.
// Paste code/errors from browser, hawk processes them as context.
type ClipboardBridge struct{}

// ReadClipboard returns the current clipboard content.
func (cb *ClipboardBridge) ReadClipboard() (string, error) {
	// macOS
	out, err := exec.CommandContext(context.Background(), "pbpaste").Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// Linux (xclip)
	out, err = exec.CommandContext(context.Background(), "xclip", "-selection", "clipboard", "-o").Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// Linux (xsel)
	out, err = exec.CommandContext(context.Background(), "xsel", "--clipboard", "--output").Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	return "", err
}

// WriteClipboard sets the clipboard content.
func (cb *ClipboardBridge) WriteClipboard(content string) error {
	// macOS
	cmd := exec.CommandContext(context.Background(), "pbcopy")
	cmd.Stdin = strings.NewReader(content)
	if err := cmd.Run(); err == nil {
		return nil
	}
	// Linux (xclip)
	cmd = exec.CommandContext(context.Background(), "xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

// IsCode heuristically detects if clipboard content is code vs prose.
func (cb *ClipboardBridge) IsCode(content string) bool {
	codeIndicators := []string{"{", "}", "()", "func ", "def ", "class ", "import ", "const ", "let ", "var ", "=>", "->", "::"}
	lines := strings.Split(content, "\n")
	codeLines := 0
	for _, line := range lines {
		for _, ind := range codeIndicators {
			if strings.Contains(line, ind) {
				codeLines++
				break
			}
		}
	}
	return float64(codeLines)/float64(len(lines)) > 0.3
}
