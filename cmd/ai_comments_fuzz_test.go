package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzScanForAIComments feeds arbitrary file contents to the AI-comment
// scanner and asserts it never panics on malformed input. The scanner reads
// source files from disk, so each fuzz input is written to a temp .go file and
// scanned. It additionally checks a weak idempotence property: re-scanning the
// same unchanged tree yields the same number of directives.
func FuzzScanForAIComments(f *testing.F) {
	seeds := []string{
		"// AI! do the thing",
		"# AI? should we?",
		"/* AI! refactor */",
		"-- AI! sql comment",
		"package main\n// AI! one\n// AI? two\n",
		"",
		"\n\n\n",
		"// AI!",                              // empty instruction
		"// AI! " + strings.Repeat("x", 4096), // very long instruction
		"// ai! lowercase should not match",
		"//AI!tight",
		"/* AI! unterminated",
		string([]byte{0x00, 0x01, 0xff, '\n', '/', '/', ' ', 'A', 'I', '!'}),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, content string) {
		dir := t.TempDir()
		path := filepath.Join(dir, "fuzz.go")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			// FIXME: test skipped in FuzzScanForAIComments
			// FIXME: fuzz test requires writing to temp dir, skip if filesystem is read-only
			t.Skip()
		}

		// Must never panic regardless of file content.
		got := scanForAIComments(dir, nil)

		// Weak idempotence: scanning the same unchanged tree again must yield
		// the same count and equivalent directives.
		again := scanForAIComments(dir, nil)
		if len(got) != len(again) {
			t.Fatalf("non-idempotent scan: first=%d second=%d", len(got), len(again))
		}
		for i := range got {
			if got[i] != again[i] {
				t.Fatalf("non-idempotent directive at %d: %+v vs %+v", i, got[i], again[i])
			}
		}

		// formatDirectivesAsPrompt must also tolerate any scan output.
		_ = formatDirectivesAsPrompt(got)
	})
}

// BenchmarkScanForAIComments measures the hot path of walking a source tree
// and matching AI directives line by line. It builds a small synthetic tree
// once and re-scans it each iteration.
func BenchmarkScanForAIComments(b *testing.B) {
	dir := b.TempDir()

	var sb strings.Builder
	sb.WriteString("package main\n")
	for i := 0; i < 200; i++ {
		sb.WriteString("x := compute() // ordinary line\n")
		if i%20 == 0 {
			sb.WriteString("// AI! tighten this loop\n")
		}
		if i%37 == 0 {
			sb.WriteString("// AI? is this correct?\n")
		}
	}
	body := []byte(sb.String())

	for i := 0; i < 8; i++ {
		sub := filepath.Join(dir, "pkg"+string(rune('a'+i)))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "file.go"), body, 0o644); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scanForAIComments(dir, nil)
	}
}
