package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestValidateBrowserURL(t *testing.T) {
	table := []struct {
		url   string
		valid bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1:8080", true},
		{"ftp://example.com", false},
		{"file:///etc/passwd", false},
		{"data:text/html,hi", false},
		{"", false},
		{"https://", false},
		{"https://example.com/path", true},
	}
	for _, tc := range table {
		err := validateBrowserURL(tc.url)
		if (err == nil) != tc.valid {
			t.Errorf("validateBrowserURL(%q) = %v, want valid=%v", tc.url, err, tc.valid)
		}
	}
}

func TestTruncateChars(t *testing.T) {
	if got := truncateChars("hello world", 5); got != "hello…" {
		t.Errorf("got %q", got)
	}
	if got := truncateChars("short", 100); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncateChars("  padded  ", 100); got != "padded" {
		t.Errorf("got %q", got)
	}
}

func TestBrowserToolSchemaAndParams(t *testing.T) {
	var bt BrowserTool
	if bt.Name() != "Browser" {
		t.Fatalf("wrong name %q", bt.Name())
	}
	if bt.RiskLevel() != "high" {
		t.Fatalf("expected high risk, got %q", bt.RiskLevel())
	}
	params := bt.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing properties")
	}
	for _, key := range []string{"action", "url", "selector", "text", "path", "wait_ms"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing param %q", key)
		}
	}
	req, ok := params["required"].([]string)
	if !ok || len(req) != 1 || req[0] != "action" {
		t.Errorf("unexpected required: %v", req)
	}

	// Execute must reject an empty action without touching a browser.
	if _, err := bt.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Errorf("expected error for missing action")
	}
	if _, err := bt.Execute(context.Background(), json.RawMessage(`{"action":"bogus","url":"https://example.com"}`)); err == nil {
		t.Errorf("expected error for unknown action")
	}
	// Unknown action with a non-http url should surface the url validation error.
	if _, err := bt.Execute(context.Background(), json.RawMessage(`{"action":"navigate","url":"file:///etc/passwd"}`)); err == nil {
		t.Errorf("expected error for non-http url")
	}
}

func TestScreenshotToolSchema(t *testing.T) {
	var st ScreenshotTool
	if st.Name() != "Screenshot" {
		t.Fatalf("wrong name %q", st.Name())
	}
	params := st.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing properties")
	}
	if _, ok := props["url"]; !ok {
		t.Errorf("missing url param")
	}
	req, ok := params["required"].([]string)
	if !ok || len(req) != 1 || req[0] != "url" {
		t.Errorf("unexpected required: %v", req)
	}
	// Reject non-http URLs before any browser interaction.
	if _, err := st.Execute(context.Background(), json.RawMessage(`{"url":"file:///tmp/x"}`)); err == nil {
		t.Errorf("expected error for non-http url")
	}
}
