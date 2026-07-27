package session

import (
	"path/filepath"
	"testing"
)

func TestLastN(t *testing.T) {
	tests := []struct {
		input    []string
		n        int
		expected []string
	}{
		{[]string{"a", "b", "c"}, 2, []string{"b", "c"}},
		{[]string{"a", "b", "c"}, 5, []string{"a", "b", "c"}},
		{[]string{}, 3, []string{}},
		{[]string{"a"}, 0, []string{}},
	}

	for _, tt := range tests {
		result := lastN(tt.input, tt.n)
		if len(result) != len(tt.expected) {
			t.Errorf("lastN(%v, %d) = %v (len %d), want %v (len %d)", tt.input, tt.n, result, len(result), tt.expected, len(tt.expected))
		}
	}
}

func TestLastN_ExactLength(t *testing.T) {
	input := []string{"a", "b", "c"}
	result := lastN(input, 3)
	if len(result) != 3 {
		t.Errorf("lastN with n=len should return all, got %d", len(result))
	}
}

func TestCoherenceTracker_ClassifyAct(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)

	tests := []struct {
		message  string
		expected ConversationalAct
	}{
		{"yes", ActConfirm},
		{"yeah", ActConfirm},
		{"nope", ActCorrect},
		{"that's wrong", ActCorrect},
		{"forget that", ActPivot},
		{"what if", ActExplore},
		{"and also", ActElaborate},
		{"how does this work?", ActQuestion},
		{"what is this?", ActQuestion},
		{"random message", ActUnknown},
	}

	for _, tt := range tests {
		result := ct.ClassifyAct(tt.message)
		if result != tt.expected {
			t.Errorf("ClassifyAct(%q) = %v, want %v", tt.message, result, tt.expected)
		}
	}
}

func TestCoherenceTracker_UpdateIntent(t *testing.T) {
	ct := NewCoherenceTracker(10, 5)
	ct.UpdateIntent("yes, that's correct", 1)

	state := ct.GetState()
	if state.CurrentAct != ActConfirm {
		t.Errorf("CurrentAct = %v, want %v", state.CurrentAct, ActConfirm)
	}
}

func TestConversationGraph_Empty(t *testing.T) {
	g, err := OpenConversationGraph(filepath.Join(t.TempDir(), "graph.json"), "test-session")
	if err != nil {
		t.Fatalf("OpenConversationGraph error: %v", err)
	}
	if !g.Empty() {
		t.Error("expected empty graph to be empty")
	}
}

func TestConversationGraph_SetHead_NonExistentNode(t *testing.T) {
	g, err := OpenConversationGraph(filepath.Join(t.TempDir(), "graph.json"), "test-session")
	if err != nil {
		t.Fatalf("OpenConversationGraph error: %v", err)
	}
	err = g.SetHead("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestNewUserProvenance(t *testing.T) {
	p := NewUserProvenance()
	if p.Source != ProvenanceExternalUser {
		t.Errorf("Source = %v, want %v", p.Source, ProvenanceExternalUser)
	}
	if !p.Trusted {
		t.Error("expected Trusted to be true")
	}
}

func TestNewSystemProvenance(t *testing.T) {
	p := NewSystemProvenance()
	if p.Source != ProvenanceInternalSystem {
		t.Errorf("Source = %v, want %v", p.Source, ProvenanceInternalSystem)
	}
	if !p.Trusted {
		t.Error("expected Trusted to be true")
	}
}

func TestNewInterSessionProvenance(t *testing.T) {
	p := NewInterSessionProvenance("session-123")
	if p.Source != ProvenanceInterSession {
		t.Errorf("Source = %v, want %v", p.Source, ProvenanceInterSession)
	}
	if p.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", p.SessionID, "session-123")
	}
	if !p.Trusted {
		t.Error("expected Trusted to be true")
	}
}

func TestSmartCheckpointer_FormatTriggers(t *testing.T) {
	store := NewSnapshotStore("test-session")
	sc := NewSmartCheckpointer(store)
	result := sc.FormatTriggers()
	if result == "" {
		t.Error("expected non-empty FormatTriggers result")
	}
}
