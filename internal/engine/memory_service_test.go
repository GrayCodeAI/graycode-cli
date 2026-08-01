package engine

import (
	"context"
	"testing"
)

func TestMemoryServiceRecallContextFallsBackToRecaller(t *testing.T) {
	mem := &mockMemoryRecaller{}
	service := NewMemoryService(nil).WithMemory(mem)

	got := service.RecallContext(context.Background(), "question", 128)
	if got != "## Relevant Memories\nrecalled: question" {
		t.Fatalf("RecallContext() = %q", got)
	}
}

func TestMemoryServiceRecallContextIsEmptyWithoutBackends(t *testing.T) {
	if got := NewMemoryService(nil).RecallContext(context.Background(), "question", 128); got != "" {
		t.Fatalf("RecallContext() = %q, want empty", got)
	}
}
