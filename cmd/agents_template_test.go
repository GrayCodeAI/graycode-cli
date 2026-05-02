package cmd

import (
	"os"
	"strings"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestDetectAgentsProjectType_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/go.mod", []byte("module test"), 0o644)
	chdir(t, dir)
	if got := detectAgentsProjectType(); got != "go" {
		t.Fatalf("expected go, got %s", got)
	}
}

func TestDetectAgentsProjectType_Node(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/package.json", []byte("{}"), 0o644)
	chdir(t, dir)
	if got := detectAgentsProjectType(); got != "node" {
		t.Fatalf("expected node, got %s", got)
	}
}

func TestDetectAgentsProjectType_Python(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/pyproject.toml", []byte(""), 0o644)
	chdir(t, dir)
	if got := detectAgentsProjectType(); got != "python" {
		t.Fatalf("expected python, got %s", got)
	}
}

func TestDetectAgentsProjectType_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/Cargo.toml", []byte(""), 0o644)
	chdir(t, dir)
	if got := detectAgentsProjectType(); got != "rust" {
		t.Fatalf("expected rust, got %s", got)
	}
}

func TestDetectAgentsProjectType_Generic(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := detectAgentsProjectType(); got != "generic" {
		t.Fatalf("expected generic, got %s", got)
	}
}

func TestGenerateAgentsTemplate_Go(t *testing.T) {
	out := GenerateAgentsTemplate("go")
	if !strings.Contains(out, "go test") {
		t.Fatal("missing 'go test'")
	}
	if !strings.Contains(out, "Go") {
		t.Fatal("missing 'Go'")
	}
}

func TestGenerateAgentsTemplate_Generic(t *testing.T) {
	out := GenerateAgentsTemplate("generic")
	if !strings.Contains(out, "FILL IN") {
		t.Fatal("generic template should contain FILL IN placeholder")
	}
}

func TestGenerateAgentsTemplate_AllTypes(t *testing.T) {
	for _, typ := range []string{"go", "node", "python", "rust", "generic"} {
		out := GenerateAgentsTemplate(typ)
		if out == "" {
			t.Fatalf("empty template for type %s", typ)
		}
	}
}
