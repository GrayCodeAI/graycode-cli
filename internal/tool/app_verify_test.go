package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppVerifyDetectAction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := AppVerifyTool{}.Execute(context.Background(), json.RawMessage(`{"action":"detect","path":"`+dir+`"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var recipe map[string]interface{}
	if err := json.Unmarshal([]byte(out), &recipe); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if recipe["ecosystem"] != "go" {
		t.Fatalf("ecosystem = %v", recipe["ecosystem"])
	}
}

func TestAppVerifyManifestActionPersistsContract(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := AppVerifyTool{}.Execute(context.Background(), json.RawMessage(`{"action":"manifest","path":"`+dir+`"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var resp struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Source != "detected now" || !strings.Contains(resp.Path, "environment.json") {
		t.Fatalf("resp = %+v", resp)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}

	// Second run loads the existing manifest.
	out2, err := AppVerifyTool{}.Execute(context.Background(), json.RawMessage(`{"action":"manifest","path":"`+dir+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var resp2 struct {
		Source string
	}
	if err := json.Unmarshal([]byte(out2), &resp2); err != nil {
		t.Fatal(err)
	}
	if resp2.Source != "loaded existing" {
		t.Fatalf("source = %q, want loaded existing", resp2.Source)
	}
}

func TestAppVerifySmokeSkipsWithoutStartCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := AppVerifyTool{}.Execute(context.Background(), json.RawMessage(
		`{"action":"smoke","path":"`+dir+`","readiness_seconds":2}`,
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var res smokeResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// A go library has no start command: the tool must skip cleanly rather
	// than fail or hang.
	if res.Status != "skipped" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
}

func TestAppVerifyInvalidAction(t *testing.T) {
	tool := AppVerifyTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"nope"}`)); err == nil {
		t.Fatal("expected error for unsupported action")
	}
}
