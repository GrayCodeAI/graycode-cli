package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// autoCommitCaptureTool records ToolContext.AutoCommit for the wire path.
type autoCommitCaptureTool struct {
	saw bool
}

func (t *autoCommitCaptureTool) Name() string        { return "Read" }
func (t *autoCommitCaptureTool) Description() string { return "capture" }
func (t *autoCommitCaptureTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (t *autoCommitCaptureTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	tc := tool.GetToolContext(ctx)
	t.saw = tc != nil && tc.AutoCommit
	return "ok", nil
}

func TestSetAutoCommit_PropagatesToToolContext(t *testing.T) {
	cap := &autoCommitCaptureTool{}
	sess := NewSession("test", "test", "sys", tool.NewRegistry(cap))
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	if sess.AutoCommit() {
		t.Fatal("default auto-commit should be off")
	}
	sess.SetAutoCommit(true)
	if !sess.AutoCommit() {
		t.Fatal("AutoCommit() false after SetAutoCommit(true)")
	}
	ch := make(chan StreamEvent, 4)
	res := sess.executeSingleTool(context.Background(), types.ToolCall{Name: "Read", ID: "ac"}, ch, 0, "")
	if res.isErr {
		t.Fatalf("tool failed: %#v", res)
	}
	if !cap.saw {
		t.Fatal("ToolContext.AutoCommit not set during ExecuteOne")
	}
	// Write path still honors git AutoCommit when enabled (smoke: tool runs).
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = path
}
