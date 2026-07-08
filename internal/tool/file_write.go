package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type FileWriteTool struct{}

func (FileWriteTool) Name() string      { return "Write" }
func (FileWriteTool) RiskLevel() string { return "medium" }
func (FileWriteTool) Aliases() []string { return []string{"file_write"} }
func (FileWriteTool) Description() string {
	return "Create or overwrite a file with the given content."
}

func (FileWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":      map[string]interface{}{"type": "string", "description": "File path to write"},
			"file_path": map[string]interface{}{"type": "string", "description": "Archive-compatible alias for path"},
			"content":   map[string]interface{}{"type": "string", "description": "File content"},
		},
	}
}

func (FileWriteTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	path := p.Path
	if path == "" {
		path = p.FilePath
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := validatePathAllowed(ctx, path); err != nil {
		return "", err
	}
	if reason := IsSensitivePath(path); reason != "" {
		return "", fmt.Errorf("blocked: %s", reason)
	}
	if tc := GetToolContext(ctx); tc != nil && tc.Protected != nil && tc.Protected.IsProtected(path) {
		return "", fmt.Errorf("path %s is protected (read-only)", path)
	}
	if cred := DetectCredentials(p.Content); cred != "" {
		return "", fmt.Errorf("content contains a credential (%s) — refusing to write", cred)
	}
	// Backup existing file before overwriting
	if _, statErr := os.Stat(path); statErr == nil {
		if _, backupErr := BackupFile(path); backupErr != nil {
			// Log the backup failure so the user knows the original may
			// be lost on a bad write. Previously this was silently dropped.
			slog.Warn("file write: backup failed, proceeding with overwrite", "path", path, "error", backupErr)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	// Write atomically: temp file in the same directory → sync → rename.
	// This prevents file corruption if the process crashes mid-write,
	// which os.WriteFile (truncate-then-write) cannot guarantee.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hawk-write-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // cleanup if rename fails

	if _, err := tmp.Write([]byte(p.Content)); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return "", fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("rename: %w", err)
	}
	if autoCommitEnabled(ctx) {
		_ = AutoCommit(ctx, path, "Write", "wrote file")
	}
	lintNote := postWriteLint(ctx, path)
	return fmt.Sprintf("Wrote %d bytes to %s%s", len(p.Content), path, lintNote), nil
}
