package engine

import (
	"os"
	"testing"
	"time"

	analytics "github.com/GrayCodeAI/hawk/internal/observability"
)

func TestCostTracker_NewAndRecord(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	ct := NewCostTracker("test-session")
	if ct == nil {
		t.Fatal("NewCostTracker returned nil")
	}

	err := ct.Record(analytics.CostEntry{
		Model:        "claude-sonnet",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.018,
		Timestamp:    time.Now(),
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestCostTracker_SessionTotal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	ct := NewCostTracker("total-test")
	_ = ct.Record(analytics.CostEntry{CostUSD: 0.01, Timestamp: time.Now()})
	_ = ct.Record(analytics.CostEntry{CostUSD: 0.02, Timestamp: time.Now()})

	total := ct.SessionTotal()
	if total < 0.029 || total > 0.031 {
		t.Errorf("SessionTotal() = %f, want ~0.03", total)
	}
}

func TestCostTracker_Entries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	ct := NewCostTracker("entries-test")
	_ = ct.Record(analytics.CostEntry{Model: "m1", CostUSD: 0.01, Timestamp: time.Now()})
	_ = ct.Record(analytics.CostEntry{Model: "m2", CostUSD: 0.02, Timestamp: time.Now()})

	entries := ct.Entries()
	if len(entries) != 2 {
		t.Errorf("Entries() = %d, want 2", len(entries))
	}
}

func TestLoadCostHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk", 0o755)

	ct := NewCostTracker("history-test")
	_ = ct.Record(analytics.CostEntry{Model: "test", CostUSD: 0.05, Timestamp: time.Now()})

	history, err := LoadCostHistory()
	if err != nil {
		t.Fatalf("LoadCostHistory: %v", err)
	}
	_ = history
}

func TestSplitJSONLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int
	}{
		{`{"a":1}` + "\n" + `{"b":2}` + "\n", 2},
		{"", 0},
		{`{"single":true}` + "\n", 1},
	}
	for _, tt := range tests {
		lines := splitJSONLines([]byte(tt.input))
		if len(lines) != tt.want {
			t.Errorf("splitJSONLines(%q) = %d, want %d", tt.input, len(lines), tt.want)
		}
	}
}
