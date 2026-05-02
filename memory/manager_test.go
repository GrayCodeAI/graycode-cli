package memory

import (
	"testing"
)

func TestNewMemoryManager(t *testing.T) {
	mm := NewMemoryManager(t.TempDir())
	if mm == nil {
		t.Fatal("expected non-nil MemoryManager")
	}
	if mm.Auto == nil {
		t.Fatal("Auto subsystem not initialized")
	}
	if mm.Evolving == nil {
		t.Fatal("Evolving subsystem not initialized")
	}
	if mm.Zen == nil {
		t.Fatal("Zen subsystem not initialized")
	}
	if mm.Yaad == nil {
		t.Fatal("Yaad subsystem not initialized")
	}
}

func TestMemoryManager_Remember(t *testing.T) {
	mm := NewMemoryManager(t.TempDir())
	categories := []string{"guideline", "core", "procedural", "fact", "session", "other"}
	for _, cat := range categories {
		if err := mm.Remember("test content for "+cat, cat); err != nil {
			t.Fatalf("Remember(%q) error: %v", cat, err)
		}
	}
}

func TestMemoryManager_Recall(t *testing.T) {
	mm := NewMemoryManager(t.TempDir())
	// Store something via ZenBrain so Recall can find it.
	mm.Zen.Store(LayerCore, "always use gofmt", []string{"preference"})

	result, err := mm.Recall("gofmt", 1000)
	if err != nil {
		t.Fatalf("Recall error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty recall result")
	}
}

func TestMemoryManager_FormatForPrompt(t *testing.T) {
	mm := NewMemoryManager(t.TempDir())
	// Should not panic even with empty subsystems.
	_ = mm.FormatForPrompt()
}
