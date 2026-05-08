package sight

import (
	"context"
	"testing"

	sightLib "github.com/GrayCodeAI/sight"
)

// mockProvider implements sight.Provider for testing.
type mockProvider struct {
	response string
}

func (m *mockProvider) Chat(_ context.Context, msgs []sightLib.Message, _ sightLib.ChatOpts) (*sightLib.Response, error) {
	return &sightLib.Response{Content: m.response, TokensUsed: 10}, nil
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
	if result.Report != "sight bridge not initialized" {
		t.Fatalf("expected fallback report, got: %s", result.Report)
	}
}

func TestBridge_DescribeNotReady(t *testing.T) {
	b := &Bridge{}
	desc, err := b.Describe(context.Background(), "diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Title != "sight bridge not initialized" {
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
	// Test that EyrieAdapter correctly translates sight messages.
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
