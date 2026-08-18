package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type declaredTimeoutTool struct{ d time.Duration }

func (declaredTimeoutTool) Name() string        { return "DeclaredTimeout" }
func (declaredTimeoutTool) Description() string { return "declares an execution budget" }
func (declaredTimeoutTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (declaredTimeoutTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	return "ok", nil
}
func (t declaredTimeoutTool) Timeout() time.Duration { return t.d }

type plainTool struct{}

func (plainTool) Name() string        { return "Plain" }
func (plainTool) Description() string { return "no declared budget" }
func (plainTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (plainTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	return "ok", nil
}

func TestTimeoutOf(t *testing.T) {
	t.Run("declared budget is returned", func(t *testing.T) {
		if got := TimeoutOf(declaredTimeoutTool{d: 75 * time.Millisecond}); got != 75*time.Millisecond {
			t.Fatalf("TimeoutOf = %v, want 75ms", got)
		}
	})
	t.Run("zero declaration falls through", func(t *testing.T) {
		if got := TimeoutOf(declaredTimeoutTool{d: 0}); got != 0 {
			t.Fatalf("TimeoutOf = %v, want 0", got)
		}
	})
	t.Run("undeclaring tool falls through", func(t *testing.T) {
		if got := TimeoutOf(plainTool{}); got != 0 {
			t.Fatalf("TimeoutOf = %v, want 0", got)
		}
	})
}
