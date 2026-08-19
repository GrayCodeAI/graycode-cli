package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/sessionquery"
)

func TestSessionQueryTool_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HAWK_STATE_DIR", tmpDir)
	sessDir := filepath.Join(tmpDir, "sessions")
	_ = os.MkdirAll(sessDir, 0o755)

	dbPath := filepath.Join(tmpDir, "tool_session_query.db")
	svc, err := sessionquery.NewService(dbPath, sessDir)
	if err != nil {
		t.Fatalf("failed to create sessionquery service: %v", err)
	}
	defer func() { _ = svc.Close() }()

	sess := &session.Session{
		ID:       "tool-sess-1",
		Model:    "claude-3-7-sonnet",
		Provider: "anthropic",
		CWD:      tmpDir,
		Messages: []session.Message{
			{Role: "user", Content: "How to configure OpenTelemetry collector with Prometheus?"},
			{Role: "assistant", Content: "Configure otel-collector.yaml with prometheus exporter."},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := session.Save(sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	qTool := SessionQueryTool{Service: svc}
	ctx := context.Background()

	// 1. Search for OpenTelemetry
	in, _ := json.Marshal(map[string]interface{}{
		"query":     "OpenTelemetry",
		"workspace": tmpDir,
	})
	out, err := qTool.Execute(ctx, in)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, "tool-sess-1") {
		t.Fatalf("output %q should contain session ID tool-sess-1", out)
	}
	if !strings.Contains(out, "OpenTelemetry") {
		t.Fatalf("output %q should contain OpenTelemetry", out)
	}

	// 2. Empty query returns error
	inEmpty, _ := json.Marshal(map[string]interface{}{
		"query": "",
	})
	_, err = qTool.Execute(ctx, inEmpty)
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	// 3. No matches returns friendly response
	inNoMatch, _ := json.Marshal(map[string]interface{}{
		"query": "nonexistenttermxyz123",
	})
	outNoMatch, err := qTool.Execute(ctx, inNoMatch)
	if err != nil {
		t.Fatalf("Execute no match failed: %v", err)
	}
	if !strings.Contains(outNoMatch, "No matching messages found") {
		t.Fatalf("expected 'No matching messages found', got %s", outNoMatch)
	}
}
