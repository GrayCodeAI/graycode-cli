package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// stubTool is a trivial read-only tool used to exercise the batch path without
// depending on the full registry of real tools.
type stubTool struct{}

func (stubTool) Name() string        { return "StubRead" }
func (stubTool) Description() string { return "test read-only tool" }
func (stubTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}
func (stubTool) Execute(_ context.Context, _ json.RawMessage) (string, error) { return "ok", nil }

// batchRegistry builds a registry containing a canonical read-only name that
// IsReadOnly recognizes, backed by the test stub.
func batchRegistry() *Registry {
	r := NewRegistry()
	_ = r.Register(stubTool{})
	// Alias the stub under the canonical read-only "Read" name so the guard
	// passes in tests without importing the whole tool set.
	_ = r.Register(readAlias{inner: stubTool{}})
	return r
}

// readAlias presents stubTool under the read-only "Read" name.
type readAlias struct{ inner Tool }

func (a readAlias) Name() string        { return "Read" }
func (a readAlias) Aliases() []string   { return []string{"read"} }
func (a readAlias) Description() string { return a.inner.Description() }
func (a readAlias) Parameters() map[string]interface{} {
	return a.inner.Parameters()
}

func (a readAlias) Execute(ctx context.Context, in json.RawMessage) (string, error) {
	return a.inner.Execute(ctx, in)
}

func TestBatchExecutesReadOnlyCalls(t *testing.T) {
	reg := batchRegistry()
	ctx := WithToolContext(context.Background(), &ToolContext{Registry: reg})

	var b BatchTool
	out, err := b.Execute(ctx, []byte(`{"calls":[{"tool":"Read","input":{}},{"tool":"Read","input":{}}]}`))
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if !strings.Contains(out, "## Read") || !strings.Contains(out, "ok") {
		t.Fatalf("unexpected batch output: %q", out)
	}
}

func TestBatchRejectsNonReadOnly(t *testing.T) {
	reg := batchRegistry()
	ctx := WithToolContext(context.Background(), &ToolContext{Registry: reg})

	var b BatchTool
	_, err := b.Execute(ctx, []byte(`{"calls":[{"tool":"Write","input":{}}]}`))
	if err == nil {
		t.Fatal("batch must reject a non-read-only tool")
	}
}

func TestBatchRejectsEmptyAndUnknown(t *testing.T) {
	reg := batchRegistry()
	ctx := WithToolContext(context.Background(), &ToolContext{Registry: reg})
	var b BatchTool

	if _, err := b.Execute(ctx, []byte(`{"calls":[]}`)); err == nil {
		t.Fatal("empty calls must error")
	}
	if _, err := b.Execute(ctx, []byte(`{"calls":[{"tool":"NoSuchTool","input":{}}]}`)); err == nil {
		t.Fatal("unknown tool must error")
	}
}
