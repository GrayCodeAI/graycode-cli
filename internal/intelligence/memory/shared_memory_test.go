package memory

import (
	"testing"
)

func TestIsContradiction(t *testing.T) {
	tests := []struct {
		existing string
		incoming string
		want     bool
	}{
		{"Use jest for testing", "Don't use jest for testing", true},
		{"Always use strict mode", "Never use strict mode", true},
		{"tabs over spaces", "spaces over tabs", true},
		{"Use jest for testing", "Use jest for unit testing", false},
		{"Enable dark mode", "Disable dark mode", true},
		{"Use React", "Use Vue", false}, // different subjects
	}

	for _, tt := range tests {
		got := isContradiction(tt.existing, tt.incoming)
		if got != tt.want {
			t.Errorf("isContradiction(%q, %q) = %v, want %v", tt.existing, tt.incoming, got, tt.want)
		}
	}
}

func TestExtractSubject(t *testing.T) {
	tests := []struct {
		text   string
		prefix string
		want   string
	}{
		{"always use strict mode", "always ", "use strict mode"},
		{"never commit directly", "never ", "commit directly"},
		{"use jest for testing", "use ", "jest for testing"},
	}

	for _, tt := range tests {
		got := extractSubject(tt.text, tt.prefix)
		if got != tt.want {
			t.Errorf("extractSubject(%q, %q) = %q, want %q", tt.text, tt.prefix, got, tt.want)
		}
	}
}

func TestSharedMemoryNilBridge(t *testing.T) {
	bridge := &HarrierBridge{ready: false}
	sm := NewSharedMemory(bridge, "mission-1", "agent-a")

	err := sm.Share("test content", "convention")
	if err != nil {
		t.Errorf("expected nil error for not-ready bridge, got %v", err)
	}

	result, err := sm.Recall("test", 500)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestSharedMemoryEvents(t *testing.T) {
	bridge := &HarrierBridge{ready: false}
	sm := NewSharedMemory(bridge, "mission-1", "agent-a")

	var received []MemoryEvent
	sm.OnMemoryEvent(func(e MemoryEvent) {
		received = append(received, e)
	})

	// No events should fire when bridge is not ready
	sm.Share("test", "convention")
	if len(received) != 0 {
		t.Errorf("expected no events when bridge not ready, got %d", len(received))
	}
}
