package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// copyResult describes where the copied content ended up.
type copyResult struct {
	FallbackPath string // non-empty when clipboard was unavailable and content was written to a file
}

// copyToClipboard copies text to the system clipboard.
// Uses pbcopy on macOS, xclip on Linux, clip.exe on Windows.
// On failure, falls back to writing the text to a file in the state dir.
func copyToClipboard(text string) copyResult {
	if err := copyToClipboardNative(text); err == nil {
		return copyResult{}
	}
	// Clipboard tool not available (common in SSH, containers, headless).
	// Fall back to writing the content to a file so the user can recover it.
	if path, err := copyToFallbackFile(text); err == nil {
		return copyResult{FallbackPath: path}
	}
	return copyResult{}
}

// copyToClipboardNative attempts to copy text to the system clipboard
// using platform-native tools. Returns an error if no tool is available
// or if the copy fails.
func copyToClipboardNative(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbcopy")
	case "linux":
		// Try wl-copy (Wayland), then xclip, then xsel.
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.CommandContext(ctx, "wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("clipboard not available: install wl-copy, xclip, or xsel")
		}
	case "windows":
		cmd = exec.CommandContext(ctx, "clip.exe")
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// copyToFallbackFile writes the given text to a timestamped file in the
// hawk state directory when the system clipboard is unavailable. This
// ensures the user can still recover copied content from SSH sessions,
// containers, or headless environments.
func copyToFallbackFile(text string) (string, error) {
	dir := storage.StateDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("clipboard unavailable and fallback write failed: %w", err)
	}
	name := fmt.Sprintf("clipboard-%d.txt", time.Now().UnixNano())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", fmt.Errorf("clipboard unavailable and fallback write failed: %w", err)
	}
	return path, nil
}

// pasteFromClipboard reads text from the system clipboard.
// Uses pbpaste on macOS, xclip on Linux, powershell on Windows.
func pasteFromClipboard() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbpaste")
	case "linux":
		// Try wl-paste (Wayland), then xclip, then xsel.
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.CommandContext(ctx, "wl-paste", "--no-newline")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, "xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("clipboard not available: install wl-paste, xclip, or xsel")
		}
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-command", "Get-Clipboard")
	default:
		return "", fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("clipboard read failed: %w", err)
	}
	return out.String(), nil
}
