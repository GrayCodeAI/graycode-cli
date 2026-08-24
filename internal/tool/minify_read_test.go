package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWithMinifyStripsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	src := `// header
package demo

// doc
func Add(a, b int) int {
	// inline
	return a + b
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	in, _ := json.Marshal(map[string]interface{}{"path": path, "minify": true})
	out, err := FileReadTool{}.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "//") || strings.Contains(out, "header") {
		t.Fatalf("minified read still contains comments:\n%s", out)
	}
	if !strings.Contains(out, "func Add") {
		t.Fatalf("minified read lost code:\n%s", out)
	}
}

func TestReadWithoutMinifyKeepsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	src := "// keep\npackage demo\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	in, _ := json.Marshal(map[string]string{"path": path})
	out, err := FileReadTool{}.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "// keep") {
		t.Fatalf("plain read should keep comments:\n%s", out)
	}
}
