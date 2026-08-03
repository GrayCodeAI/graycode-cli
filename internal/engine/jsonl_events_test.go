package engine

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONLEventWriter_ContentAndDone(t *testing.T) {
	var buf bytes.Buffer
	w := NewJSONLEventWriter(&buf)

	if err := w.Content(1, "hello"); err != nil {
		t.Fatalf("Content: %v", err)
	}
	if err := w.Done(1, "end_turn"); err != nil {
		t.Fatalf("Done: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}

	var first, second map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 0 not JSON: %v", err)
	}
	if first["type"] != "content" {
		t.Errorf("line 0 type = %v, want content", first["type"])
	}
	if first["turn"].(float64) != 1 {
		t.Errorf("line 0 turn = %v, want 1", first["turn"])
	}
	if sd, _ := first["data"].(map[string]any); sd["value"] != "hello" {
		t.Errorf("line 0 data.value = %v, want hello", sd["value"])
	}

	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("line 1 not JSON: %v", err)
	}
	if second["type"] != "done" {
		t.Errorf("line 1 type = %v, want done", second["type"])
	}
}

func TestJSONLEventWriter_DoesNotInterleave(t *testing.T) {
	// Concurrency-safe: many goroutines emitting concurrently must produce
	// whole, parseable JSON lines with no interleaving.
	var buf bytes.Buffer
	w := NewJSONLEventWriter(&buf)

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(n int) {
			_ = w.Content(n, "body")
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(lines))
	}
	for i, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %d not valid JSON (interleaving?): %v", i, err)
		}
	}
}
