package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectVerifyDetectsStacks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := (ProjectVerifyTool{}).Execute(context.Background(), mustProjectJSON(map[string]interface{}{"action": "detect", "path": dir}))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, `"go"`) || !strings.Contains(out, `"go.mod"`) {
		t.Fatalf("detection output = %s", out)
	}
}

func TestProjectVerifyRejectsUnknownAction(t *testing.T) {
	_, err := (ProjectVerifyTool{}).Execute(context.Background(), json.RawMessage(`{"action":"unknown"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("unknown action error = %v", err)
	}
}

func TestBoundedCommandOutputCapsNoisyChecks(t *testing.T) {
	var output boundedCommandOutput
	input := bytes.Repeat([]byte("x"), verificationOutputLimit+1)
	if written, err := output.Write(input); err != nil || written != len(input) {
		t.Fatalf("Write() = (%d, %v), want all input accepted", written, err)
	}
	if !output.Truncated() {
		t.Fatal("expected noisy output to be marked truncated")
	}
	if len(output.String()) != verificationOutputLimit {
		t.Fatalf("output length = %d, want %d", len(output.String()), verificationOutputLimit)
	}
}

func mustProjectJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
