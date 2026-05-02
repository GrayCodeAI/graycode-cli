package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportContext_Basic(t *testing.T) {
	// Use a temp directory with a go.mod to simulate a Go project
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)

	result, err := ExportContext(dir, "")
	if err != nil {
		t.Fatalf("ExportContext error: %v", err)
	}

	if !strings.Contains(result, "# Project Context") {
		t.Error("expected '# Project Context' header")
	}
	if !strings.Contains(result, "Go module") {
		t.Error("expected Go module detection")
	}
	if !strings.Contains(result, "## Directory Structure") {
		t.Error("expected directory structure section")
	}
	if !strings.Contains(result, "go.mod") {
		t.Error("expected go.mod in key files")
	}
}

func TestExportContextToFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)

	outPath := filepath.Join(t.TempDir(), "context.md")
	err := ExportContextToFile(dir, "", outPath)
	if err != nil {
		t.Fatalf("ExportContextToFile error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file error: %v", err)
	}
	if !strings.Contains(string(data), "Node.js project") {
		t.Error("expected Node.js project detection in output file")
	}
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		file     string
		wantType string
		wantLang string
	}{
		{"go.mod", "Go module", "Go"},
		{"Cargo.toml", "Rust crate", "Rust"},
		{"package.json", "Node.js project", "JavaScript/TypeScript"},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, tt.file), []byte("test"), 0o644)
		gotType, gotLang := detectProjectType(dir)
		if gotType != tt.wantType {
			t.Errorf("detectProjectType(%s): type = %q, want %q", tt.file, gotType, tt.wantType)
		}
		if gotLang != tt.wantLang {
			t.Errorf("detectProjectType(%s): lang = %q, want %q", tt.file, gotLang, tt.wantLang)
		}
	}
}

func TestDirTree(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte(""), 0o644)

	tree := dirTree(dir, 2)
	if !strings.Contains(tree, "src/") {
		t.Error("expected 'src/' in tree output")
	}
	if !strings.Contains(tree, "README.md") {
		t.Error("expected 'README.md' in tree output")
	}
}

func TestExportContext_WithFocus(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "engine"), 0o755)
	os.WriteFile(filepath.Join(dir, "engine", "core.go"), []byte("package engine\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0o644)

	result, err := ExportContext(dir, "engine")
	if err != nil {
		t.Fatalf("ExportContext with focus error: %v", err)
	}
	if !strings.Contains(result, "Focus Area: engine") {
		t.Error("expected focus area section")
	}
}
