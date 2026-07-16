package tool

import (
	"testing"
)

func TestNewPromptQueueItem(t *testing.T) {
	item := NewPromptQueueItem("test prompt", "Test Subject")
	if item.Prompt != "test prompt" {
		t.Errorf("Prompt = %q, want %q", item.Prompt, "test prompt")
	}
	if item.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want %q", item.Subject, "Test Subject")
	}
	if item.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestPromptQueueStateCount(t *testing.T) {
	tests := []struct {
		name     string
		state    *PromptQueueState
		expected int
	}{
		{"empty", &PromptQueueState{Items: nil}, 0},
		{"nil items", &PromptQueueState{}, 0},
		{"one item", &PromptQueueState{Items: []PromptQueueItem{{}}}, 1},
		{"multiple items", &PromptQueueState{Items: []PromptQueueItem{{}, {}, {}}}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.Count()
			if got != tt.expected {
				t.Errorf("Count() = %d, want %d", got, tt.expected)
			}
		})
	}
}
