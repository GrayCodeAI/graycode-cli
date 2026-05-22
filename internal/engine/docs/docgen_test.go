package docs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocGenerator_Generate_Basic(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	dg := NewDocGenerator(dir)
	doc, err := dg.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if doc == nil {
		t.Fatal("doc is nil")
	}
	if doc.Name != filepath.Base(dir) {
		t.Errorf("Name = %q", doc.Name)
	}
}

func TestDocGenerator_Generate_WithPackages(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pkg1"), 0o755)
	os.WriteFile(filepath.Join(dir, "pkg1", "stuff.go"), []byte("package pkg1\n\nfunc Foo() {}\n"), 0o644)

	dg := NewDocGenerator(dir)
	doc, err := dg.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(doc.Packages) == 0 {
		t.Fatal("expected packages")
	}
}

func TestRenderMarkdown(t *testing.T) {
	doc := &ProjectDoc{Name: "test", Description: "a test"}
	out := RenderMarkdown(doc)
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestRenderHTML(t *testing.T) {
	doc := &ProjectDoc{Name: "test", Description: "a test"}
	out := RenderHTML(doc)
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestGenerateREADME(t *testing.T) {
	doc := &ProjectDoc{Name: "test", Description: "a test"}
	out := GenerateREADME(doc)
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestDocGenerator_InferDescription(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# MyProject\n\nThis is a test project.\n"), 0o644)

	dg := NewDocGenerator(dir)
	desc := dg.InferDescription(dir)
	if desc == "" {
		t.Error("expected non-empty description")
	}
}

func TestDocGenerator_NonExistentDir(t *testing.T) {
	dg := NewDocGenerator("/nonexistent/path")
	_, err := dg.Generate()
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestDocGenerator_FileAsProjectDir(t *testing.T) {
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "file.txt")
	os.WriteFile(tmpFile, []byte("not a dir"), 0o644)

	dg := NewDocGenerator(tmpFile)
	_, err := dg.Generate()
	if err == nil {
		t.Error("expected error when project path is a file")
	}
}
