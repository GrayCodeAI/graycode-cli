package cmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns the
// captured bytes.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()
	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	_ = r.Close()
	return string(buf[:n])
}

func TestPrintTextUsageFooter(t *testing.T) {
	started := time.Now().Add(-2 * time.Second)

	t.Run("renders token, turns, model and elapsed summary", func(t *testing.T) {
		got := captureStderr(t, func() {
			printTextUsageFooter(&engine.StreamUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			}, started, 3, "graycode-pro")
		})
		if !strings.Contains(got, "100 in · 50 out") {
			t.Errorf("footer missing token counts: %q", got)
		}
		if !strings.Contains(got, "tokens:") {
			t.Errorf("footer missing prefix: %q", got)
		}
		if !strings.Contains(got, "3 turn(s)") {
			t.Errorf("footer missing turn count: %q", got)
		}
		if !strings.Contains(got, "graycode-pro") {
			t.Errorf("footer missing model: %q", got)
		}
		if !strings.Contains(got, "2s") {
			t.Errorf("footer missing elapsed: %q", got)
		}
	})

	t.Run("includes cache when nonzero", func(t *testing.T) {
		got := captureStderr(t, func() {
			printTextUsageFooter(&engine.StreamUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				CacheReadTokens:  90,
				CacheWriteTokens: 7,
			}, started, 1, "")
		})
		if !strings.Contains(got, "cache 90 read · 7 write") {
			t.Errorf("footer missing cache summary: %q", got)
		}
	})

	t.Run("omits cache when zero", func(t *testing.T) {
		got := captureStderr(t, func() {
			printTextUsageFooter(&engine.StreamUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
			}, started, 1, "")
		})
		if strings.Contains(got, "cache") {
			t.Errorf("footer should omit zero cache: %q", got)
		}
	})

	t.Run("omits model when empty", func(t *testing.T) {
		got := captureStderr(t, func() {
			printTextUsageFooter(&engine.StreamUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
			}, started, 1, "")
		})
		if strings.Contains(got, "graycode-pro") {
			t.Errorf("footer should omit empty model: %q", got)
		}
	})

	t.Run("skips output when usage is nil", func(t *testing.T) {
		got := captureStderr(t, func() {
			printTextUsageFooter(nil, started, 1, "m")
		})
		if got != "" {
			t.Errorf("expected no output for nil usage, got: %q", got)
		}
	})

	t.Run("skips output when quiet is set", func(t *testing.T) {
		old := quietFlag
		quietFlag = true
		defer func() { quietFlag = old }()
		got := captureStderr(t, func() {
			printTextUsageFooter(&engine.StreamUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			}, started, 1, "m")
		})
		if got != "" {
			t.Errorf("expected no output under --quiet, got: %q", got)
		}
	})
}
