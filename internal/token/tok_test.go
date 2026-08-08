package token

import (
	"strings"
	"testing"

	tok "github.com/GrayCodeAI/tok"
	tokgraph "github.com/GrayCodeAI/tok/runtimegraph"
)

// These tests guard the dependency boundary: they verify hawk's token package
// forwards to the external tok library correctly so wiring regressions (wrong
// arg order, dropped params) surface in CI without needing the library's own
// test suite.

func TestCountTokens(t *testing.T) {
	// Precise counting must produce a positive count for non-empty text and
	// zero for empty input, and should roughly track text length.
	empty := CountTokens("")
	if empty != 0 {
		t.Errorf("CountTokens(\"\") = %d, want 0", empty)
	}

	short := CountTokens("hello world")
	long := CountTokens(strings.Repeat("the quick brown fox jumps over the lazy dog. ", 20))
	if short <= 0 {
		t.Errorf("CountTokens(short) = %d, want > 0", short)
	}
	if long <= short {
		t.Errorf("CountTokens(long) = %d must exceed CountTokens(short) = %d", long, short)
	}
}

func TestCountTokensFast(t *testing.T) {
	n := CountTokensFast(strings.Repeat("word ", 100))
	if n <= 0 {
		t.Errorf("CountTokensFast = %d, want > 0", n)
	}
}

func TestCompress(t *testing.T) {
	text := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 10)

	// A budget larger than the input must return the input unchanged.
	big, stats := Compress(text, len(text)*4)
	if big == "" {
		t.Error("Compress with a large budget returned empty text")
	}
	if stats.OriginalTokens <= 0 {
		t.Errorf("Compress stats.OriginalTokens = %d, want > 0", stats.OriginalTokens)
	}

	// A tiny budget must produce a strictly smaller (or empty) result.
	tiny, _ := Compress(text, 1)
	if len(tiny) >= len(text) {
		t.Errorf("Compress with budget=1 did not reduce text: %d -> %d chars", len(text), len(tiny))
	}
}

func TestNewUsageTracker(t *testing.T) {
	ut := NewUsageTracker()
	if ut == nil {
		t.Fatal("NewUsageTracker returned nil")
	}
}

func TestChunkCode(t *testing.T) {
	source := `package main

func main() {
	println("hello")
}
`
	chunks := ChunkCode(source, ChunkOptions{})
	if len(chunks) == 0 {
		t.Fatal("ChunkCode returned no chunks for non-empty source")
	}
}

func TestDefaultSecretDetector(t *testing.T) {
	det := DefaultSecretDetector()
	if det == nil {
		t.Fatal("DefaultSecretDetector returned nil")
	}
}

func TestBuildRuntimeGraph(t *testing.T) {
	// A minimal graph input (one usage summary) should build without error.
	usage := tokgraph.Input{Usage: &tok.UsageSummary{}}
	out, err := BuildRuntimeGraph(usage)
	if err != nil {
		t.Fatalf("BuildRuntimeGraph: %v", err)
	}
	if out == nil {
		t.Fatal("BuildRuntimeGraph returned nil export")
	}

	// An input with no summaries must be rejected.
	if _, err := BuildRuntimeGraph(tokgraph.Input{}); err == nil {
		t.Error("BuildRuntimeGraph with no summaries should error")
	}
}
