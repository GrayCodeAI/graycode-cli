package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildSyntheticRepo creates a temp repo tree with nFiles Go files, each
// carrying a handful of functions and types, so benchmarks exercise symbol
// extraction without depending on a real checkout.
func buildSyntheticRepo(b *testing.B, nFiles int) string {
	dir := b.TempDir()
	for i := 0; i < nFiles; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("pkg%d", i%8))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		path := filepath.Join(sub, fmt.Sprintf("file%d.go", i))
		var sb strings.Builder
		sb.WriteString("package p\n")
		for f := 0; f < 20; f++ {
			fmt.Fprintf(&sb, "// Func%d does a thing.\nfunc Func%d(a, b int) int { return a + b }\n", f, f)
		}
		for t := 0; t < 5; t++ {
			fmt.Fprintf(&sb, "type Type%d struct { A int; B string }\n", t)
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

// BenchmarkRepoMapGenerate measures end-to-end repo-map generation over a
// synthetic 100-file tree and reports the token estimate and formatted size
// (Gap-04 P0: repomap size/tokens).
func BenchmarkRepoMapGenerate(b *testing.B) {
	dir := buildSyntheticRepo(b, 100)
	var tokEst, outLen int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rm, err := Generate(dir, Options{MaxFiles: 500, MaxTokens: 2000})
		if err != nil {
			b.Fatal(err)
		}
		out := rm.Format(2000)
		tokEst = rm.TokenEst
		outLen = len(out)
	}
	b.ReportMetric(float64(tokEst), "est_tokens")
	b.ReportMetric(float64(outLen), "format_bytes")
}
