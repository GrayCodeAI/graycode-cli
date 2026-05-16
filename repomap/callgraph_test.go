package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		skip bool
	}{
		{"vendor", true},
		{"node_modules", true},
		{".git", true},
		{"src", false},
		{"internal", false},
		{"pkg", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if shouldSkipDir(tt.name) != tt.skip {
				t.Errorf("shouldSkipDir(%q) = %v, want %v", tt.name, !tt.skip, tt.skip)
			}
		})
	}
}

func TestAppendUniqueStr_Callgraph(t *testing.T) {
	t.Parallel()
	s := []string{"a", "b"}
	s = appendUniqueStr(s, "c")
	if len(s) != 3 {
		t.Errorf("len = %d, want 3", len(s))
	}
	s = appendUniqueStr(s, "b")
	if len(s) != 3 {
		t.Errorf("duplicate should not be added, len = %d", len(s))
	}
}

func TestBuildCallGraph(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal Go project
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() { helper() }
func helper() {}
`), 0o644)

	cg, err := BuildCallGraph(dir)
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if cg == nil {
		t.Fatal("callgraph should not be nil")
	}
}

func TestCallGraph_CallersOf(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() { helper() }
func helper() { utility() }
func utility() {}
`), 0o644)

	cg, _ := BuildCallGraph(dir)
	callers := cg.CallersOf("helper", 2)
	_ = callers
}

func TestCallGraph_CalleesOf(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() { a(); b() }
func a() {}
func b() { c() }
func c() {}
`), 0o644)

	cg, _ := BuildCallGraph(dir)
	callees := cg.CalleesOf("main", 2)
	_ = callees
}

func TestCallGraph_Neighborhood(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() { x() }
func x() { y() }
func y() {}
`), 0o644)

	cg, _ := BuildCallGraph(dir)
	neighbors := cg.Neighborhood("x", 1)
	_ = neighbors
}
