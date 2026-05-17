package engine

import (
	"strings"
	"testing"
)

func TestNewBeliefState(t *testing.T) {
	bs := NewBeliefState()
	if bs == nil {
		t.Fatal("expected non-nil belief state")
	}
	if bs.Size() != 0 {
		t.Errorf("expected 0 beliefs, got %d", bs.Size())
	}
}

func TestBeliefState_RecordAndGet(t *testing.T) {
	bs := NewBeliefState()

	bs.Record("file_purpose", "engine.go", "Contains the agent loop, handles streaming + tool execution", 1)
	bs.Record("file_purpose", "session.go", "Manages persistence with WAL + atomic writes", 2)
	bs.Record("dependency", "engine.go", "Depends on client package for LLM interaction", 3)

	if bs.Size() != 3 {
		t.Errorf("expected 3 beliefs, got %d", bs.Size())
	}

	// Get beliefs about engine.go
	beliefs := bs.Get("engine.go")
	if len(beliefs) != 2 {
		t.Fatalf("expected 2 beliefs about engine.go, got %d", len(beliefs))
	}

	// Get beliefs about a subject with no beliefs
	empty := bs.Get("nonexistent.go")
	if len(empty) != 0 {
		t.Errorf("expected 0 beliefs for nonexistent subject, got %d", len(empty))
	}
}

func TestBeliefState_RecordUpdatesExisting(t *testing.T) {
	bs := NewBeliefState()

	bs.Record("file_purpose", "main.go", "Entry point", 1)
	bs.Record("file_purpose", "main.go", "Entry point with CLI handling", 5)

	// Should still be 1 belief (updated, not duplicated)
	if bs.Size() != 1 {
		t.Errorf("expected 1 belief after update, got %d", bs.Size())
	}

	beliefs := bs.Get("main.go")
	if len(beliefs) != 1 {
		t.Fatalf("expected 1 belief, got %d", len(beliefs))
	}
	if beliefs[0].Content != "Entry point with CLI handling" {
		t.Errorf("expected updated content, got %q", beliefs[0].Content)
	}
	if beliefs[0].LastVerified != 5 {
		t.Errorf("expected LastVerified=5, got %d", beliefs[0].LastVerified)
	}
}

func TestBeliefState_FormatForPrompt(t *testing.T) {
	bs := NewBeliefState()

	// Empty state should return empty string
	empty := bs.FormatForPrompt()
	if empty != "" {
		t.Errorf("expected empty string for no beliefs, got %q", empty)
	}

	bs.Record("file_purpose", "engine.go", "Contains the agent loop", 1)
	bs.Record("file_purpose", "session.go", "Manages persistence", 2)
	bs.Record("architecture", "engine", "Uses streaming for LLM communication", 3)

	prompt := bs.FormatForPrompt()
	if !strings.Contains(prompt, "What you know so far:") {
		t.Errorf("prompt should start with header, got: %s", prompt)
	}
	if !strings.Contains(prompt, "engine.go") {
		t.Errorf("prompt should mention engine.go, got: %s", prompt)
	}
	if !strings.Contains(prompt, "session.go") {
		t.Errorf("prompt should mention session.go, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Contains the agent loop") {
		t.Errorf("prompt should include belief content, got: %s", prompt)
	}
}

func TestBeliefState_Invalidate(t *testing.T) {
	bs := NewBeliefState()

	bs.Record("file_purpose", "config.go", "Handles configuration", 1)
	bs.Record("dependency", "config.go", "Depends on yaml package", 2)
	bs.Record("file_purpose", "main.go", "Entry point", 3)

	// Invalidate config.go beliefs
	bs.Invalidate("config.go")

	configBeliefs := bs.Get("config.go")
	for _, b := range configBeliefs {
		if b.Confidence >= 1.0 {
			t.Errorf("invalidated belief %q should have reduced confidence, got %f", b.ID, b.Confidence)
		}
		if b.Confidence != 0.5 {
			t.Errorf("expected confidence=0.5 after invalidation, got %f", b.Confidence)
		}
	}

	// main.go beliefs should be unaffected
	mainBeliefs := bs.Get("main.go")
	for _, b := range mainBeliefs {
		if b.Confidence != 1.0 {
			t.Errorf("unrelated belief should have confidence=1.0, got %f", b.Confidence)
		}
	}
}

func TestBeliefState_Prune(t *testing.T) {
	bs := NewBeliefState()

	bs.Record("file_purpose", "old.go", "Old file purpose", 1)
	bs.Record("file_purpose", "recent.go", "Recent file purpose", 20)
	bs.Record("architecture", "old_arch", "Old architecture note", 5)

	// Prune at turn 30: beliefs not verified since turn 10 (30-20) should be removed
	bs.Prune(30)

	// old.go (verified at turn 1) and old_arch (verified at turn 5) should be pruned
	// recent.go (verified at turn 20) should remain
	if bs.Size() != 1 {
		t.Errorf("expected 1 belief after prune, got %d", bs.Size())
	}

	remaining := bs.Get("recent.go")
	if len(remaining) != 1 {
		t.Errorf("expected recent.go to survive prune, got %d beliefs", len(remaining))
	}

	gone := bs.Get("old.go")
	if len(gone) != 0 {
		t.Errorf("expected old.go to be pruned, got %d beliefs", len(gone))
	}
}
