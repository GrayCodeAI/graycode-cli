package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSemanticIndex(t *testing.T) {
	dir := t.TempDir()

	// Create some Go files
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "util.go"), []byte("package main\n\nfunc helper() string {\n\treturn \"helper result\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildSemanticIndex(dir, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Size() == 0 {
		t.Fatal("expected non-empty index")
	}
}

func TestSemanticSearch(t *testing.T) {
	dir := t.TempDir()

	// Create files with distinct content
	if err := os.WriteFile(filepath.Join(dir, "http.go"), []byte("package main\n\nfunc handleHTTPRequest() {\n\t// process incoming HTTP request\n\t// parse headers and body\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db.go"), []byte("package main\n\nfunc queryDatabase() {\n\t// execute SQL query against database\n\t// return results from database\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildSemanticIndex(dir, nil, 100)
	if err != nil {
		t.Fatal(err)
	}

	results := idx.Search("database query", 5)
	if len(results) == 0 {
		t.Fatal("expected search results")
	}
	// The database-related chunk should rank first
	if results[0].Path != "db.go" {
		t.Fatalf("expected db.go as top result, got %s", results[0].Path)
	}
}

func TestSemanticSearchEmpty(t *testing.T) {
	idx := &SemanticIndex{}
	results := idx.Search("anything", 5)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty index, got %d", len(results))
	}
}

func TestSemanticIndexSaveLoad(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n\nfunc test() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildSemanticIndex(dir, nil, 100)
	if err != nil {
		t.Fatal(err)
	}

	savePath := filepath.Join(dir, "index.gob")
	if err := idx.Save(savePath); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSemanticIndex(savePath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Size() != idx.Size() {
		t.Fatalf("loaded index size %d != original %d", loaded.Size(), idx.Size())
	}
}

func TestSemanticIndexSize(t *testing.T) {
	dir := t.TempDir()

	// Create a file with more than 40 lines to get multiple chunks
	var content string
	for i := 0; i < 100; i++ {
		content += "// line of code\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte("package main\n"+content), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildSemanticIndex(dir, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Size() < 2 {
		t.Fatalf("expected at least 2 chunks for 100+ lines, got %d", idx.Size())
	}
}
