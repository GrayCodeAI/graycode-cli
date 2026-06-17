package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ExampleGenerate is a worked example for the repomap package: it builds a
// tiny project under a temp directory, calls Generate, and renders the
// resulting RepoMap with Format. The output is verified to be non-empty
// and to contain the expected file and symbol names.
//
// This example is consumed by `go doc` and `godoc` to illustrate the
// package's primary entry point.
func ExampleGenerate() {
	dir, err := os.MkdirTemp("", "repomap-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// Two supported files: one Go, one Python. Both have a function
	// and a type declaration so the example exercises the AST and
	// regex paths.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}

type Server struct{}
`), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte(`class App:
    pass

def run():
    pass
`), 0o644); err != nil {
		panic(err)
	}

	rm, err := Generate(dir, Options{MaxFiles: 100, MaxTokens: 5000})
	if err != nil {
		panic(err)
	}

	out := rm.Format(5000)
	if out == "" {
		panic("expected non-empty output from Format")
	}
}

// TestExampleGenerate_isRunnable mirrors ExampleGenerate with assertions so
// `go test` validates the example end-to-end. The example function above
// has no // Output: comment (the output depends on the temp directory
// path, which is not stable across runs), so this test is the actual
// validator; godoc still picks up ExampleGenerate for the docs page.
func TestExampleGenerate_isRunnable(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}

type Server struct{}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte(`class App:
    pass

def run():
    pass
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rm, err := Generate(dir, Options{MaxFiles: 100, MaxTokens: 5000})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if rm == nil {
		t.Fatal("expected non-nil RepoMap")
	}
	if len(rm.Files) != 2 {
		t.Fatalf("expected 2 file maps, got %d", len(rm.Files))
	}

	out := rm.Format(5000)
	if out == "" {
		t.Fatal("expected non-empty formatted output")
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("expected main.go in output, got:\n%s", out)
	}
	if !strings.Contains(out, "app.py") {
		t.Errorf("expected app.py in output, got:\n%s", out)
	}
	// The Go file should have a func main and a type Server.
	if !strings.Contains(out, "main") {
		t.Errorf("expected main symbol in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Server") {
		t.Errorf("expected Server symbol in output, got:\n%s", out)
	}
}
