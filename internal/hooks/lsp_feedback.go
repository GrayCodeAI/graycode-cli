package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// LSPFeedbackProvider abstracts LSP diagnostics for the feedback hook.
// This interface allows the hook to call into the LSP manager without
// creating an import cycle.
type LSPFeedbackProvider interface {
	// DiagnosticsForFile returns diagnostics for a file path.
	// Returns nil if no LSP server is available for the file type.
	DiagnosticsForFile(ctx context.Context, filePath string) ([]Diagnostic, error)
}

// Diagnostic mirrors lsp.Diagnostic to avoid import cycle.
type Diagnostic struct {
	Line     int
	Column   int
	Severity int // 1=error, 2=warning, 3=info, 4=hint
	Code     string
	Message  string
}

// fileEditTools lists tool names that modify files.
var fileEditTools = map[string]bool{
	"Write":      true,
	"Edit":       true,
	"MultiEdit":  true,
	"file_write": true,
	"file_edit":  true,
}

// LSPFeedbackHook creates a hook that runs LSP diagnostics after file edits
// and provides blocking feedback when errors are found.
func LSPFeedbackHook(provider LSPFeedbackProvider) Hook {
	return Hook{
		Name:     "lsp-feedback",
		Event:    EventPostTool,
		Priority: 100,
		FnV2: func(ctx context.Context, envelope EventEnvelope) error {
			return handleLSPFeedback(ctx, provider, envelope)
		},
	}
}

func handleLSPFeedback(ctx context.Context, provider LSPFeedbackProvider, envelope EventEnvelope) error {
	if provider == nil {
		return nil
	}

	toolName, _ := envelope.Payload["tool"].(string)
	if !fileEditTools[toolName] {
		return nil
	}

	filePath := extractEditedFilePath(envelope.Payload)
	if filePath == "" {
		return nil
	}

	// Skip non-source files
	ext := strings.ToLower(filepath.Ext(filePath))
	skipExts := map[string]bool{
		".md": true, ".txt": true, ".json": true, ".yaml": true, ".yml": true,
		".toml": true, ".xml": true, ".html": true, ".css": true, ".svg": true,
		".png": true, ".jpg": true, ".gif": true, ".lock": true, ".sum": true,
	}
	if skipExts[ext] {
		return nil
	}

	diagnostics, err := provider.DiagnosticsForFile(ctx, filePath)
	if err != nil {
		slog.Debug("lsp-feedback: diagnostics failed", "file", filePath, "error", err)
		return nil // fail-open
	}

	// Filter to errors only (severity 1)
	var errors []Diagnostic
	for _, d := range diagnostics {
		if d.Severity == 1 {
			errors = append(errors, d)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	// Format blocking feedback
	var lines []string
	lines = append(lines, fmt.Sprintf("LSP found %d error(s) in %s:", len(errors), filepath.Base(filePath)))
	for _, e := range errors {
		code := ""
		if e.Code != "" {
			code = " (" + e.Code + ")"
		}
		lines = append(lines, fmt.Sprintf("  - Line %d: %s%s", e.Line, e.Message, code))
	}
	lines = append(lines, "Fix these errors before proceeding.")

	feedback := strings.Join(lines, "\n")

	// Store in payload for the tool executor to pick up
	envelope.Payload["lsp_feedback"] = feedback

	return nil
}

func extractEditedFilePath(payload map[string]interface{}) string {
	for _, key := range []string{"path", "file_path", "filePath", "file", "target"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	// For MultiEdit, try to extract from args
	if args, ok := payload["args"].(map[string]interface{}); ok {
		for _, key := range []string{"path", "file_path", "filePath", "file"} {
			if v, ok := args[key].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}
