package hooks

import (
	"context"
	"strings"
	"testing"
)

type mockFeedbackProvider struct {
	diagnostics []Diagnostic
	err         error
}

func (m *mockFeedbackProvider) DiagnosticsForFile(ctx context.Context, filePath string) ([]Diagnostic, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.diagnostics, nil
}

func TestLSPFeedbackHook_FileWithErrors(t *testing.T) {
	provider := &mockFeedbackProvider{
		diagnostics: []Diagnostic{
			{Line: 42, Severity: 1, Code: "E0001", Message: "undefined: foo"},
			{Line: 57, Severity: 1, Code: "E0052", Message: "cannot use x as type string"},
			{Line: 60, Severity: 2, Code: "W0001", Message: "unused variable"},
		},
	}
	hook := LSPFeedbackHook(provider)

	envelope := EventEnvelope{
		Payload: map[string]interface{}{
			"tool": "Write",
			"path": "/project/main.go",
		},
	}

	err := hook.FnV2(context.Background(), envelope)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fb, ok := envelope.Payload["lsp_feedback"].(string)
	if !ok {
		t.Fatal("expected lsp_feedback in payload")
	}
	if !contains(fb, "2 error(s)") {
		t.Errorf("expected 2 errors in feedback, got: %s", fb)
	}
	if !contains(fb, "undefined: foo") {
		t.Error("expected first error message in feedback")
	}
	if contains(fb, "unused variable") {
		t.Error("warnings should not appear in feedback (errors only)")
	}
}

func TestLSPFeedbackHook_FileWithNoErrors(t *testing.T) {
	provider := &mockFeedbackProvider{
		diagnostics: []Diagnostic{
			{Line: 10, Severity: 2, Code: "W0001", Message: "unused variable"},
		},
	}
	hook := LSPFeedbackHook(provider)

	envelope := EventEnvelope{
		Payload: map[string]interface{}{
			"tool": "Edit",
			"path": "/project/main.go",
		},
	}

	hook.FnV2(context.Background(), envelope)
	if _, ok := envelope.Payload["lsp_feedback"]; ok {
		t.Error("should not add feedback when there are no errors")
	}
}

func TestLSPFeedbackHook_IgnoresNonEditTools(t *testing.T) {
	provider := &mockFeedbackProvider{
		diagnostics: []Diagnostic{
			{Line: 1, Severity: 1, Message: "error"},
		},
	}
	hook := LSPFeedbackHook(provider)

	for _, tool := range []string{"Read", "Bash", "Grep", "Glob"} {
		envelope := EventEnvelope{
			Payload: map[string]interface{}{
				"tool": tool,
				"path": "/project/main.go",
			},
		}
		hook.FnV2(context.Background(), envelope)
		if _, ok := envelope.Payload["lsp_feedback"]; ok {
			t.Errorf("should not add feedback for tool %q", tool)
		}
	}
}

func TestLSPFeedbackHook_IgnoresNonSourceFiles(t *testing.T) {
	provider := &mockFeedbackProvider{
		diagnostics: []Diagnostic{
			{Line: 1, Severity: 1, Message: "error"},
		},
	}
	hook := LSPFeedbackHook(provider)

	for _, ext := range []string{".md", ".json", ".yaml", ".txt", ".lock"} {
		envelope := EventEnvelope{
			Payload: map[string]interface{}{
				"tool": "Write",
				"path": "/project/file" + ext,
			},
		}
		hook.FnV2(context.Background(), envelope)
		if _, ok := envelope.Payload["lsp_feedback"]; ok {
			t.Errorf("should not add feedback for %s files", ext)
		}
	}
}

func TestLSPFeedbackHook_NilProvider(t *testing.T) {
	hook := LSPFeedbackHook(nil)
	envelope := EventEnvelope{
		Payload: map[string]interface{}{
			"tool": "Write",
			"path": "/project/main.go",
		},
	}
	if err := hook.FnV2(context.Background(), envelope); err != nil {
		t.Fatalf("nil provider should not error: %v", err)
	}
}

func TestLSPFeedbackHook_ProviderError(t *testing.T) {
	provider := &mockFeedbackProvider{
		err: context.DeadlineExceeded,
	}
	hook := LSPFeedbackHook(provider)
	envelope := EventEnvelope{
		Payload: map[string]interface{}{
			"tool": "Write",
			"path": "/project/main.go",
		},
	}
	// Should fail-open (no error)
	if err := hook.FnV2(context.Background(), envelope); err != nil {
		t.Fatalf("provider error should be swallowed (fail-open): %v", err)
	}
}

func TestExtractEditedFilePath(t *testing.T) {
	tests := []struct {
		payload map[string]interface{}
		want    string
	}{
		{map[string]interface{}{"path": "/a.go"}, "/a.go"},
		{map[string]interface{}{"file_path": "/b.go"}, "/b.go"},
		{map[string]interface{}{"filePath": "/c.go"}, "/c.go"},
		{map[string]interface{}{"target": "/d.go"}, "/d.go"},
		{map[string]interface{}{"args": map[string]interface{}{"path": "/e.go"}}, "/e.go"},
		{map[string]interface{}{"tool": "Bash"}, ""},
	}
	for _, tt := range tests {
		got := extractEditedFilePath(tt.payload)
		if got != tt.want {
			t.Errorf("extractEditedFilePath(%v) = %q, want %q", tt.payload, got, tt.want)
		}
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
