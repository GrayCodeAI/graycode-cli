package token

import (
	"strings"
	"testing"

	shrike "github.com/GrayCodeAI/shrike"
	shrikegraph "github.com/GrayCodeAI/shrike/runtimegraph"
)

// These tests guard the dependency boundary: they verify graycode's token package
// forwards to the external shrike library correctly so wiring regressions (wrong
// arg order, dropped params) surface in CI without needing the library's own
// test suite.

// requireShrike skips the test when the shrike engine is the build-harness
// stub, which returns zeros/identity for every operation. The boundary tests
// below assert real shrike behavior, so they are meaningful only when a real
// shrike is linked in.
func requireShrike(t *testing.T) {
	t.Helper()
	if !ShrikeAvailable() {
		t.Skip("shrike engine is the build-harness stub; skipping shrike boundary test")
	}
}

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

func TestFallbackEstimate(t *testing.T) {
	// The self-contained estimator must be strictly positive for non-empty
	// text, zero for empty, monotonic in length, and land in the 3-7
	// chars-per-token band the rest of the codebase expects.
	if got := fallbackEstimate(""); got != 0 {
		t.Errorf("fallbackEstimate(\"\") = %d, want 0", got)
	}
	short := fallbackEstimate("hello world")
	if short <= 0 {
		t.Errorf("fallbackEstimate(short) = %d, want > 0", short)
	}
	long := fallbackEstimate(strings.Repeat("the quick brown fox jumps over the lazy dog. ", 20))
	if long <= short {
		t.Errorf("fallbackEstimate(long) = %d must exceed short = %d", long, short)
	}
	content := "this is a test string that should produce some tokens"
	got := fallbackEstimate(content)
	if cpt := float64(len(content)) / float64(got); cpt < 3 || cpt > 7 {
		t.Errorf("chars-per-token %0.2f outside expected range (3-7) for %d chars and %d tokens", cpt, len(content), got)
	}
}

func TestCompress(t *testing.T) {
	requireShrike(t)
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
	requireShrike(t)
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
	requireShrike(t)
	// A minimal graph input (one usage summary) should build without error.
	usage := shrikegraph.Input{Usage: &shrike.UsageSummary{}}
	out, err := BuildRuntimeGraph(usage)
	if err != nil {
		t.Fatalf("BuildRuntimeGraph: %v", err)
	}
	if out == nil {
		t.Fatal("BuildRuntimeGraph returned nil export")
	}

	// An input with no summaries must be rejected.
	if _, err := BuildRuntimeGraph(shrikegraph.Input{}); err == nil {
		t.Error("BuildRuntimeGraph with no summaries should error")
	}
}
