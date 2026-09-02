package merlin

import (
	"context"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
)

func TestNewBridge(t *testing.T) {
	b := NewBridge()
	if !b.Ready() {
		t.Fatal("expected bridge to be ready after NewBridge()")
	}
}

func TestBridge_RunNotReady(t *testing.T) {
	b := &Bridge{}
	report, err := b.Run(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Target != "https://example.com" {
		t.Fatalf("expected target in fallback report, got: %s", report.Target)
	}
}

func TestBridge_Ready(t *testing.T) {
	b := &Bridge{}
	if b.Ready() {
		t.Fatal("expected uninitialized bridge to not be ready")
	}
	b2 := NewBridge()
	if !b2.Ready() {
		t.Fatal("expected initialized bridge to be ready")
	}
}

func TestRunContractsObservedRecordsQualityGraph(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	observedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	b := &Bridge{}

	report, err := b.RunContractsObserved(
		context.Background(),
		"https://private.example",
		GraphObservation{
			SessionID:  "session-1",
			Scope:      graphcontracts.Scope{RepositoryID: "repo-1"},
			ObservedAt: observedAt,
		},
	)
	if err != nil {
		t.Fatalf("RunContractsObserved() error = %v", err)
	}
	if report == nil {
		t.Fatal("expected verification report")
	}
	entries, err := graphjournal.Load("session-1")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Quality == nil || entries[1].Verification == nil {
		t.Fatalf("entries = %#v, want quality and verification", entries)
	}
	if len(entries[0].Quality.Nodes) != 1 {
		t.Fatalf("quality nodes = %d, want report node", len(entries[0].Quality.Nodes))
	}
	if entries[1].Verification.TargetSHA256 == "" {
		t.Fatal("verification target digest is empty")
	}
}
