package kestrel

import (
	"context"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
)

func TestReviewContractsObservedRecordsQualityGraph(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	bridge := &Bridge{}
	at := time.Date(2026, time.July, 25, 13, 0, 0, 0, time.UTC)
	result, err := bridge.ReviewContractsObserved(
		context.Background(),
		"private source diff",
		GraphObservation{
			SessionID:  "session-1",
			Scope:      graphcontracts.Scope{RepositoryID: "repo-1"},
			ObservedAt: at,
		},
	)
	if err != nil {
		t.Fatalf("ReviewContractsObserved() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected review result")
	}
	entries, err := graphjournal.Load("session-1")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Quality == nil || entries[1].Verification == nil {
		t.Fatalf("entries = %#v, want quality and verification", entries)
	}
	if len(entries[0].Quality.Nodes) != 1 {
		t.Fatalf("quality nodes = %d, want review node", len(entries[0].Quality.Nodes))
	}
	if entries[1].Verification.TargetSHA256 == "" {
		t.Fatal("verification target digest is empty")
	}
}

func TestNewBridge_NilClient(t *testing.T) {
	b := NewBridge(nil, "anthropic")
	if b.Ready() {
		t.Fatal("expected bridge to not be ready with nil client")
	}
}

func TestBridge_ReviewNotReady(t *testing.T) {
	b := &Bridge{}
	result, err := b.Review(context.Background(), "diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Report != "kestrel bridge not initialized" {
		t.Fatalf("expected fallback report, got: %s", result.Report)
	}
}

func TestBridge_DescribeNotReady(t *testing.T) {
	b := &Bridge{}
	desc, err := b.Describe(context.Background(), "diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Title != "kestrel bridge not initialized" {
		t.Fatalf("expected fallback title, got: %s", desc.Title)
	}
}

func TestBridge_ImproveNotReady(t *testing.T) {
	b := &Bridge{}
	result, err := b.Improve(context.Background(), "diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestEyrieAdapter_Chat(t *testing.T) {
	// Test that EyrieAdapter correctly translates kestrel messages.
	// We can't easily test with a real eyrie client, but we verify the adapter
	// struct is properly constructed.
	adapter := NewEyrieAdapter(nil, "openai")
	if adapter.provider != "openai" {
		t.Fatalf("expected provider openai, got %s", adapter.provider)
	}
	if adapter.client != nil {
		t.Fatal("expected nil client")
	}
}
