package memory

import (
	"os"
	"testing"
)

func newTestBridge(t *testing.T) *YaadBridge {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.yaad/data", 0o755)

	b := NewYaadBridge()
	if b == nil {
		t.Fatal("NewYaadBridge returned nil")
	}
	return b
}

func TestYaadBridge_Init(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: test skipped in TestYaadBridge_Init
		// FIXME: yaad bridge requires the yaad dependency to be available at runtime
		t.Skip("yaad bridge could not initialize (missing yaad dependency)")
	}
}

func TestYaadBridge_Remember(t *testing.T) {
	// FIXME: test skipped in TestYaadBridge_Remember
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: yaad dependency must be available to test remember functionality
		t.Skip("yaad not available")
	}
	err := b.Remember("test content to remember", "explicit")
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
}

// FIXME: test skipped in TestYaadBridge_Remember

func TestYaadBridge_Recall(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: yaad dependency must be available to test recall functionality
		t.Skip("yaad not available")
	}
	_ = b.Remember("golang error handling patterns", "convention")

	result, err := b.Recall("error handling", 500)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	// FIXME: test skipped in TestYaadBridge_Recall
	}
	_ = result
}

func TestYaadBridge_Close(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
		// FIXME: yaad not available
		t.Skip("yaad not available")
	}
	_ = b.store.Close()
}

func TestConfidenceTracker_WithBridge(t *testing.T) {
	b := newTestBridge(t)
	if !b.ready {
// FIXME: test skipped
		t.Skip("yaad not available")
	}

	// FIXME: test skipped in TestConfidenceTracker_WithBridge
	ct := NewConfidenceTracker(b)
	ct.RecordAccess("node-1", "node-2")
	ct.OnSessionSuccess()
	ct.Reset()
}

func TestProactiveContext_WithBridge(t *testing.T) {
	b := newTestBridge(t)
// FIXME: test skipped
	if !b.ready {
// FIXME: test skipped
		t.Skip("yaad not available")
	}

	pc := NewProactiveContext(b)
	pc.TrackFile("main.go")
	pc.TrackFiles([]string{"config.go", "handler.go"})

// FIXME: test skipped in TestProactiveContext_WithBridge

	ctx := pc.ContextForFile("main.go")
	_ = ctx

	pc.Reset()
}

func TestGraphAwareBudget_WithBridge(t *testing.T) {
// FIXME: test skipped
	b := newTestBridge(t)
// FIXME: test skipped
	if !b.ready {
// FIXME: test skipped
		t.Skip("yaad not available")
	}

	pc := NewProactiveContext(b)
	gb := NewGraphAwareBudget(b, pc)

	budget := gb.ComputeBudget([]string{"main.go"}, 0.5)
	if budget <= 0 {
		t.Errorf("budget = %d, want > 0", budget)
	}

	injection := gb.BuildInjection("fix the bug", []string{"main.go"}, 500)
	_ = injection
}
