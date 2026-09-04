package mission

import (
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestTranscriptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workers", "feat-1.jsonl")

	// Write a transcript.
	w, err := NewPersistWriter(path)
	if err != nil {
		t.Fatalf("NewPersistWriter: %v", err)
	}

	msgs := []types.EyrieMessage{
		{Role: "user", Content: "Start working on feature 1"},
		{Role: "assistant", Content: "I'll explore the codebase first."},
		{Role: "assistant", Content: "", ToolUse: []types.ToolCall{{Name: "Bash", ID: "tc1"}}},
	}
	for _, m := range msgs {
		if err := w.Write(m); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	handoff := &Handoff{CommitID: "abc123", Summary: "Done", TestsPassed: true}
	if err := w.MarkComplete(handoff); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Check completeness.
	exists, complete, err := IsTranscriptComplete(path)
	if err != nil {
		t.Fatalf("IsTranscriptComplete: %v", err)
	}
	if !exists || !complete {
		t.Fatalf("expected transcript to exist and be complete: exists=%v complete=%v", exists, complete)
	}

	// Load and verify.
	loaded, loadedHandoff, loadedComplete, err := LoadTranscript(path)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	if !loadedComplete {
		t.Fatal("expected complete=true")
	}
	if len(loaded) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(loaded))
	}
	for i, m := range loaded {
		if m.Role != msgs[i].Role || m.Content != msgs[i].Content {
			t.Fatalf("message %d mismatch: %+v vs %+v", i, m, msgs[i])
		}
	}
	if loadedHandoff == nil || loadedHandoff.CommitID != "abc123" {
		t.Fatalf("handoff mismatch: %+v", loadedHandoff)
	}
}

func TestTranscriptIncomplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workers", "feat-2.jsonl")

	w, err := NewPersistWriter(path)
	if err != nil {
		t.Fatalf("NewPersistWriter: %v", err)
	}
	if err := w.Write(types.EyrieMessage{Role: "user", Content: "Start"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	exists, complete, err := IsTranscriptComplete(path)
	if err != nil {
		t.Fatalf("IsTranscriptComplete: %v", err)
	}
	if !exists || complete {
		t.Fatalf("expected incomplete transcript: exists=%v complete=%v", exists, complete)
	}
}

func TestTranscriptMissing(t *testing.T) {
	exists, complete, err := IsTranscriptComplete("/nonexistent/path.jsonl")
	if err != nil {
		t.Fatalf("IsTranscriptComplete: %v", err)
	}
	if exists || complete {
		t.Fatal("expected missing transcript")
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"feat-1":      "feat-1",
		"feat/1":      "feat_1",
		"feat: auth":  "feat__auth",
		"feat@oauth2": "feat_oauth2",
		"":            "feature",
	}
	for input, want := range cases {
		if got := sanitize(input); got != want {
			t.Fatalf("sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTranscriptPath(t *testing.T) {
	p := TranscriptPath("/mission/dir", "feat-1")
	want := filepath.Join("/mission/dir", "workers", "feat-1.jsonl")
	if p != want {
		t.Fatalf("TranscriptPath = %q, want %q", p, want)
	}
}

func TestWriteAndLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workers", "empty.jsonl")

	w, err := NewPersistWriter(path)
	if err != nil {
		t.Fatalf("NewPersistWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs, handoff, complete, err := LoadTranscript(path)
	if err != nil {
		t.Fatalf("LoadTranscript: %v", err)
	}
	if len(msgs) != 0 || handoff != nil || complete {
		t.Fatalf("expected empty transcript: msgs=%d handoff=%v complete=%v", len(msgs), handoff, complete)
	}
}
