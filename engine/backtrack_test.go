package engine

import (
	"strings"
	"testing"

	"github.com/hawk/eyrie/client"
)

func TestNewBacktrackEngine(t *testing.T) {
	be := NewBacktrackEngine()
	if be == nil {
		t.Fatal("expected non-nil backtrack engine")
	}
	if be.maxPoints != 50 {
		t.Errorf("expected maxPoints=50, got %d", be.maxPoints)
	}
	if be.Size() != 0 {
		t.Errorf("expected 0 decision points, got %d", be.Size())
	}
}

func TestBacktrackEngine_RecordAndFind(t *testing.T) {
	be := NewBacktrackEngine()

	msgs := []client.EyrieMessage{
		{Role: "user", Content: "Fix the bug"},
		{Role: "assistant", Content: "I'll try approach A"},
	}

	be.RecordDecision(1, "Use regex to parse HTML", []string{"Use an HTML parser", "Use string splitting"}, msgs)
	be.RecordDecision(3, "Modify config.go directly", []string{"Create a new config file"}, msgs)

	if be.Size() != 2 {
		t.Fatalf("expected 2 decision points, got %d", be.Size())
	}

	// No failures yet, FindBacktrackPoint should return nil
	bp := be.FindBacktrackPoint()
	if bp != nil {
		t.Error("expected nil backtrack point when no failures recorded")
	}

	// Mark the second decision as failed
	be.MarkOutcome(3, "failure")
	bp = be.FindBacktrackPoint()
	if bp == nil {
		t.Fatal("expected non-nil backtrack point after marking failure")
	}
	if bp.TurnIndex != 3 {
		t.Errorf("expected turn index 3, got %d", bp.TurnIndex)
	}
	if bp.Description != "Modify config.go directly" {
		t.Errorf("unexpected description: %q", bp.Description)
	}
	if len(bp.Alternatives) != 1 || bp.Alternatives[0] != "Create a new config file" {
		t.Errorf("unexpected alternatives: %v", bp.Alternatives)
	}
}

func TestBacktrackEngine_GenerateRetryPrompt(t *testing.T) {
	be := NewBacktrackEngine()

	msgs := []client.EyrieMessage{
		{Role: "user", Content: "Refactor the function"},
	}

	be.RecordDecision(5, "Inline all helper functions", []string{"Extract to a separate package", "Use interfaces"}, msgs)
	be.MarkOutcome(5, "failure")

	bp := be.FindBacktrackPoint()
	if bp == nil {
		t.Fatal("expected backtrack point")
	}

	prompt := be.GenerateRetryPrompt(bp)
	if !strings.Contains(prompt, "Previous approach failed: Inline all helper functions") {
		t.Errorf("prompt should describe the failure, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Extract to a separate package") {
		t.Errorf("prompt should mention alternatives, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Please try a different approach") {
		t.Errorf("prompt should contain retry instruction, got: %s", prompt)
	}

	// Nil decision point should return empty string
	emptyPrompt := be.GenerateRetryPrompt(nil)
	if emptyPrompt != "" {
		t.Errorf("expected empty prompt for nil decision, got: %q", emptyPrompt)
	}
}

func TestBacktrackEngine_RestoreState(t *testing.T) {
	be := NewBacktrackEngine()

	originalMsgs := []client.EyrieMessage{
		{Role: "user", Content: "Step one"},
		{Role: "assistant", Content: "Done with step one"},
		{Role: "user", Content: "Step two"},
	}

	be.RecordDecision(2, "Chose approach X", []string{"approach Y"}, originalMsgs)
	be.MarkOutcome(2, "failure")

	bp := be.FindBacktrackPoint()
	restored := be.RestoreState(bp)

	if len(restored) != len(originalMsgs) {
		t.Fatalf("expected %d messages, got %d", len(originalMsgs), len(restored))
	}

	for i, msg := range restored {
		if msg.Role != originalMsgs[i].Role || msg.Content != originalMsgs[i].Content {
			t.Errorf("message %d mismatch: got {%s, %s}, want {%s, %s}",
				i, msg.Role, msg.Content, originalMsgs[i].Role, originalMsgs[i].Content)
		}
	}

	// Verify it's a copy (mutating restored should not affect the engine)
	restored[0].Content = "mutated"
	bp2 := be.FindBacktrackPoint()
	state2 := be.RestoreState(bp2)
	if state2[0].Content == "mutated" {
		t.Error("RestoreState should return a copy, not a reference to internal state")
	}

	// Nil decision point should return nil
	nilState := be.RestoreState(nil)
	if nilState != nil {
		t.Error("expected nil for nil decision point")
	}
}
