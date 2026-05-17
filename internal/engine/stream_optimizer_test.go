package engine

import (
	"strings"
	"testing"
	"time"
)

func TestStreamOptimizer_NewDefaults(t *testing.T) {
	opt := NewStreamOptimizer()
	if opt.BufferSize != 50 {
		t.Errorf("expected BufferSize 50, got %d", opt.BufferSize)
	}
	if opt.MinFlushInterval != 16*time.Millisecond {
		t.Errorf("expected MinFlushInterval 16ms, got %v", opt.MinFlushInterval)
	}
	if !opt.DeduplicateRepeats {
		t.Error("expected DeduplicateRepeats true")
	}
	if !opt.ProgressiveRender {
		t.Error("expected ProgressiveRender true")
	}
}

func TestStreamOptimizer_SmallChunksBuffered(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.BufferSize = 50
	opt.MinFlushInterval = 1 * time.Second // long interval so only size triggers

	ch := make(chan string, 10)
	out := opt.Process(ch)

	// Send small chunks that together are below buffer size
	ch <- "he"
	ch <- "ll"
	ch <- "o"

	// Close and read output
	close(ch)

	var result []string
	for s := range out {
		result = append(result, s)
	}

	combined := strings.Join(result, "")
	if combined != "hello" {
		t.Errorf("expected 'hello', got %q", combined)
	}
}

func TestStreamOptimizer_FlushAtSizeThreshold(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.BufferSize = 10
	opt.MinFlushInterval = 10 * time.Second // disable time-based flush

	ch := make(chan string, 10)
	out := opt.Process(ch)

	// Send enough data to exceed buffer size
	ch <- "abcdefghij " // 11 chars, exceeds buffer of 10
	ch <- "end"

	close(ch)

	var result []string
	for s := range out {
		result = append(result, s)
	}

	// Should have flushed at least once before final
	if len(result) < 1 {
		t.Errorf("expected at least 1 flush, got %d", len(result))
	}

	combined := strings.Join(result, "")
	if !strings.Contains(combined, "abcdefghij") {
		t.Errorf("expected content to include 'abcdefghij', got %q", combined)
	}
}

func TestStreamOptimizer_FlushAtTimeThreshold(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.BufferSize = 1000 // large buffer so size doesn't trigger
	opt.MinFlushInterval = 20 * time.Millisecond

	ch := make(chan string, 10)
	out := opt.Process(ch)

	ch <- "hello world "

	// Wait for time-based flush
	time.Sleep(50 * time.Millisecond)

	close(ch)

	var result []string
	for s := range out {
		result = append(result, s)
	}

	combined := strings.Join(result, "")
	if combined != "hello world " {
		t.Errorf("expected 'hello world ', got %q", combined)
	}
}

func TestStreamOptimizer_StutterDetection(t *testing.T) {
	opt := NewStreamOptimizer()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"repeated suffix", "hello worldhello world", "hello world"},
		{"no repeat", "hello world", "hello world"},
		{"short input", "ab", "ab"},
		{"empty", "", ""},
		{"triple repeat", "abcabc", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opt.DetectStutter(tt.input)
			if got != tt.expect {
				t.Errorf("DetectStutter(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestStreamOptimizer_StutterDeduplication(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.BufferSize = 5
	opt.MinFlushInterval = 1 * time.Millisecond

	ch := make(chan string, 10)
	out := opt.Process(ch)

	// Send duplicate chunks (LLM stutter)
	ch <- "hello"
	ch <- "hello" // duplicate, should be dropped
	ch <- " world."

	time.Sleep(20 * time.Millisecond)
	close(ch)

	var result []string
	for s := range out {
		result = append(result, s)
	}

	combined := strings.Join(result, "")
	if combined != "hello world." {
		t.Errorf("expected 'hello world.', got %q", combined)
	}
}

func TestStreamOptimizer_IncompleteWordBuffering(t *testing.T) {
	opt := NewStreamOptimizer()

	tests := []struct {
		name      string
		input     string
		complete  string
		remainder string
	}{
		{"breaks at space", "hello world foo", "hello world ", "foo"},
		{"breaks at newline", "hello\nworld", "hello\n", "world"},
		{"breaks at period", "hello.world", "hello.", "world"},
		{"breaks at comma", "hello,world", "hello,", "world"},
		{"no break small", "abc", "", "abc"},
		{"empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			complete, remainder := opt.DetectIncomplete(tt.input)
			if complete != tt.complete {
				t.Errorf("complete = %q, want %q", complete, tt.complete)
			}
			if remainder != tt.remainder {
				t.Errorf("remainder = %q, want %q", remainder, tt.remainder)
			}
		})
	}
}

func TestStreamOptimizer_CodeFenceDetection(t *testing.T) {
	opt := NewStreamOptimizer()

	// Unclosed code fence - should not flush
	input := "here is code:\n```go\nfunc main() {"
	complete, remainder := opt.DetectIncomplete(input)
	if complete != "" {
		t.Errorf("expected no complete output with unclosed fence, got %q", complete)
	}
	if remainder != input {
		t.Errorf("expected full input as remainder, got %q", remainder)
	}

	// Closed code fence - should flush
	input2 := "```go\nfunc main() {}\n```\nmore text"
	complete2, remainder2 := opt.DetectIncomplete(input2)
	if complete2 == "" {
		t.Error("expected complete output with closed fence")
	}
	_ = remainder2
}

func TestStreamOptimizer_StatsTracking(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.BufferSize = 5
	opt.MinFlushInterval = 1 * time.Millisecond

	ch := make(chan string, 10)
	out := opt.Process(ch)

	ch <- "hello "
	ch <- "world."

	time.Sleep(30 * time.Millisecond)
	close(ch)

	// Drain output
	for range out {
	}

	stats := opt.Stats()
	if stats.TotalChars == 0 {
		t.Error("expected TotalChars > 0")
	}
	if stats.FlushCount == 0 {
		t.Error("expected FlushCount > 0")
	}
	if stats.Duration == 0 {
		t.Error("expected Duration > 0")
	}
}

func TestStreamOptimizer_WordWrap(t *testing.T) {
	opt := NewStreamOptimizer()

	tests := []struct {
		name   string
		input  string
		width  int
		expect string
	}{
		{"short line", "hello world", 80, "hello world"},
		{"wrap needed", "hello world foo bar", 11, "hello world\nfoo bar"},
		{"exact fit", "hello", 5, "hello"},
		{"zero width", "hello", 0, "hello"},
		{"multiline", "line one\nline two", 80, "line one\nline two"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opt.WordWrap(tt.input, tt.width)
			if got != tt.expect {
				t.Errorf("WordWrap(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.expect)
			}
		})
	}
}

func TestStreamOptimizer_ProgressIndicator(t *testing.T) {
	opt := NewStreamOptimizer()

	// Spinner during first 2 seconds
	indicator := opt.ProgressIndicator(500*time.Millisecond, 0)
	if indicator == "" {
		t.Error("expected spinner indicator")
	}

	// "thinking..." after 2 seconds
	indicator2 := opt.ProgressIndicator(3200*time.Millisecond, 0)
	if !strings.Contains(indicator2, "thinking...") {
		t.Errorf("expected 'thinking...' in %q", indicator2)
	}
	if !strings.Contains(indicator2, "3.2s") {
		t.Errorf("expected '3.2s' in %q", indicator2)
	}

	// chars/s while streaming
	indicator3 := opt.ProgressIndicator(2*time.Second, 284)
	if !strings.Contains(indicator3, "142 chars/s") {
		t.Errorf("expected '142 chars/s' in %q", indicator3)
	}
}

func TestStreamOptimizer_OptimizeToolOutput_PathCollapsing(t *testing.T) {
	opt := NewStreamOptimizer()

	input := "error in /home/user/projects/myapp/src/components/Button.tsx:42"
	result := opt.OptimizeToolOutput(input)

	if strings.Contains(result, "/home/user/projects/myapp") {
		t.Errorf("expected path to be collapsed, got %q", result)
	}
	if !strings.Contains(result, ".../components/Button.tsx") {
		t.Errorf("expected collapsed path with last two components, got %q", result)
	}
}

func TestStreamOptimizer_OptimizeToolOutput_RepeatedLines(t *testing.T) {
	opt := NewStreamOptimizer()

	lines := []string{
		"processing file alpha.go",
		"processing file beta.go",
		"processing file gamma.go",
		"processing file delta.go",
		"processing file epsilon.go",
		"processing file zeta.go",
		"done",
	}
	input := strings.Join(lines, "\n")

	result := opt.OptimizeToolOutput(input)

	if !strings.Contains(result, "more similar lines") {
		t.Errorf("expected repeated lines to be collapsed, got %q", result)
	}
}

func TestStreamOptimizer_OptimizeToolOutput_Empty(t *testing.T) {
	opt := NewStreamOptimizer()
	result := opt.OptimizeToolOutput("")
	if result != "" {
		t.Errorf("expected empty result for empty input, got %q", result)
	}
}

func TestStreamOptimizer_EmptyInput(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.MinFlushInterval = 1 * time.Millisecond

	ch := make(chan string, 10)
	out := opt.Process(ch)

	close(ch)

	var result []string
	for s := range out {
		result = append(result, s)
	}

	if len(result) != 0 {
		t.Errorf("expected no output for empty input, got %v", result)
	}
}

func TestStreamOptimizer_Reset(t *testing.T) {
	opt := NewStreamOptimizer()
	opt.BufferSize = 5
	opt.MinFlushInterval = 1 * time.Millisecond

	ch := make(chan string, 10)
	out := opt.Process(ch)

	ch <- "hello "
	ch <- "world."
	time.Sleep(20 * time.Millisecond)
	close(ch)

	for range out {
	}

	// Verify stats are non-zero
	stats := opt.Stats()
	if stats.TotalChars == 0 {
		t.Fatal("expected non-zero stats before reset")
	}

	opt.Reset()

	stats2 := opt.Stats()
	if stats2.TotalChars != 0 {
		t.Errorf("expected TotalChars 0 after reset, got %d", stats2.TotalChars)
	}
	if stats2.FlushCount != 0 {
		t.Errorf("expected FlushCount 0 after reset, got %d", stats2.FlushCount)
	}
	if stats2.DeduplicatedChars != 0 {
		t.Errorf("expected DeduplicatedChars 0 after reset, got %d", stats2.DeduplicatedChars)
	}
	if stats2.BufferedChars != 0 {
		t.Errorf("expected BufferedChars 0 after reset, got %d", stats2.BufferedChars)
	}
}
