package session

import (
	"strings"
	"testing"
)

func TestDiff_Identical(t *testing.T) {
	a := &Session{
		ID: "sess-a",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}
	b := &Session{
		ID: "sess-b",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}

	result := Diff(a, b)
	if !strings.Contains(result, "Sessions are identical") {
		t.Fatalf("expected identical message, got:\n%s", result)
	}
}

func TestDiff_Divergent(t *testing.T) {
	a := &Session{
		ID: "sess-a",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "branch A"},
		},
	}
	b := &Session{
		ID: "sess-b",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "branch B"},
		},
	}

	result := Diff(a, b)
	if !strings.Contains(result, "Common messages: 2") {
		t.Fatalf("expected common messages: 2, got:\n%s", result)
	}
	if !strings.Contains(result, "Diverge at index: 2") {
		t.Fatalf("expected diverge at index 2, got:\n%s", result)
	}
	if !strings.Contains(result, "branch A") {
		t.Fatalf("expected branch A in diff, got:\n%s", result)
	}
	if !strings.Contains(result, "branch B") {
		t.Fatalf("expected branch B in diff, got:\n%s", result)
	}
}

func TestDiff_DifferentLengths(t *testing.T) {
	a := &Session{
		ID: "sess-a",
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}
	b := &Session{
		ID: "sess-b",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "more"},
		},
	}

	result := Diff(a, b)
	if !strings.Contains(result, "Session B (messages 1..2)") {
		t.Fatalf("expected B-only messages, got:\n%s", result)
	}
}

func TestDiff_NilSessions(t *testing.T) {
	if result := Diff(nil, nil); !strings.Contains(result, "Both sessions are nil") {
		t.Fatalf("expected both nil message, got: %s", result)
	}
	if result := Diff(nil, &Session{ID: "b"}); !strings.Contains(result, "First session is nil") {
		t.Fatalf("expected first nil message, got: %s", result)
	}
	if result := Diff(&Session{ID: "a"}, nil); !strings.Contains(result, "Second session is nil") {
		t.Fatalf("expected second nil message, got: %s", result)
	}
}

func TestDiff_ModelDifference(t *testing.T) {
	a := &Session{ID: "a", Model: "gpt-4o", Messages: []Message{{Role: "user", Content: "hi"}}}
	b := &Session{ID: "b", Model: "claude-3", Messages: []Message{{Role: "user", Content: "hi"}}}

	result := Diff(a, b)
	if !strings.Contains(result, "Model: gpt-4o -> claude-3") {
		t.Fatalf("expected model difference, got:\n%s", result)
	}
}
