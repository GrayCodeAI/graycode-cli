package magicdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	code := `package main

// Hello says hello.
func Hello(name string) string {
	return "Hello " + name
}

// Person represents a person.
type Person struct {
	Name string
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	foundHello := false
	foundPerson := false
	for _, e := range entries {
		if e.Name == "Hello" && e.Type == "function" {
			foundHello = true
			if !strings.Contains(e.Doc, "says hello") {
				t.Fatalf("expected doc to contain 'says hello', got %q", e.Doc)
			}
		}
		if e.Name == "Person" && e.Type == "type" {
			foundPerson = true
		}
	}
	if !foundHello {
		t.Fatal("expected Hello function")
	}
	if !foundPerson {
		t.Fatal("expected Person type")
	}
}

func TestGenerateMarkdown(t *testing.T) {
	entries := []DocEntry{
		{Package: "main", Name: "Hello", Type: "function", Doc: "Says hello.\n"},
		{Package: "main", Name: "World", Type: "function", Doc: "Says world.\n"},
	}

	md := GenerateMarkdown(entries)
	if !strings.Contains(md, "# API Documentation") {
		t.Fatal("expected markdown header")
	}
	if !strings.Contains(md, "Hello") {
		t.Fatal("expected Hello in markdown")
	}
}
