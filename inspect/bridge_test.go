package inspect

import (
	"context"
	"testing"
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
