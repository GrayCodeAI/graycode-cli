package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMod writes a minimal go.mod file for testing readRequires/checkDrift.
func writeMod(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadRequires(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "go.mod")
	writeMod(t, modPath, `module example.com/test

go 1.26

require (
	github.com/GrayCodeAI/eagle v1.0.0
	github.com/spf13/cobra v1.8.0
)

require github.com/GrayCodeAI/shrike v1.9.0 // indirect
`)

	reqs, err := readRequires(modPath)
	if err != nil {
		t.Fatalf("readRequires: %v", err)
	}
	tests := map[string]string{
		"github.com/GrayCodeAI/eagle":  "v1.0.0",
		"github.com/spf13/cobra":       "v1.8.0",
		"github.com/GrayCodeAI/shrike": "v1.9.0",
	}
	for mod, want := range tests {
		if got := reqs[mod]; got != want {
			t.Errorf("reqs[%q] = %q, want %q", mod, got, want)
		}
	}
}

func TestReadRequires_MissingFile(t *testing.T) {
	if _, err := readRequires(filepath.Join(t.TempDir(), "nope.mod")); err == nil {
		t.Error("readRequires on missing file should error")
	}
}

func TestReadRequires_InvalidMod(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(bad, []byte("not a go.mod {{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRequires(bad); err == nil {
		t.Error("readRequires on invalid go.mod should error")
	}
}

// TestCheckDrift reports drift when a consumer pins an older version than graycode.
func TestCheckDrift_DetectsDrift(t *testing.T) {
	ws := t.TempDir()
	writeMod(t, filepath.Join(ws, "graycode", "go.mod"), `module github.com/GrayCodeAI/graycode-cli

go 1.26

require github.com/GrayCodeAI/falcon v1.5.0
`)
	// Consumer sibling pins an older version of the shared contract.
	writeMod(t, filepath.Join(ws, "merlin", "go.mod"), `module github.com/GrayCodeAI/merlin

go 1.26

require github.com/GrayCodeAI/falcon v1.2.0
`)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := checkDrift(filepath.Join(ws, "graycode"))
	_ = w.Close()
	os.Stdout = old
	_, _ = buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("checkDrift: %v", err)
	}
	if !strings.Contains(buf.String(), "merlin") {
		t.Errorf("expected drift report to mention merlin consumer, got:\n%s", buf.String())
	}
}

// TestCheckDrift_NoDriftWhenVersionsMatch verifies the happy path: matching
// pins produce the "OK" line and no per-consumer drift lines.
func TestCheckDrift_NoDriftWhenVersionsMatch(t *testing.T) {
	ws := t.TempDir()
	writeMod(t, filepath.Join(ws, "graycode", "go.mod"), `module github.com/GrayCodeAI/graycode-cli

go 1.26

require github.com/GrayCodeAI/falcon v1.5.0
`)
	writeMod(t, filepath.Join(ws, "kestrel", "go.mod"), `module github.com/GrayCodeAI/kestrel

go 1.26

require github.com/GrayCodeAI/falcon v1.5.0
`)

	var buf bytes.Buffer
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	err := checkDrift(filepath.Join(ws, "graycode"))
	_ = w.Close()
	os.Stdout = old
	_, _ = buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("checkDrift: %v", err)
	}
	if !strings.Contains(buf.String(), "OK — no drift") {
		t.Errorf("expected OK line for matching pins, got:\n%s", buf.String())
	}
}

// TestCheckDrift_SkipsMissingSiblings verifies that a sibling directory
// without a go.mod (e.g. not a Go module) is skipped without failing.
func TestCheckDrift_SkipsMissingSiblings(t *testing.T) {
	ws := t.TempDir()
	writeMod(t, filepath.Join(ws, "graycode", "go.mod"), `module github.com/GrayCodeAI/graycode-cli

go 1.26

require github.com/GrayCodeAI/falcon v1.5.0
`)
	// Directory present but no go.mod — must be skipped silently.
	if err := os.MkdirAll(filepath.Join(ws, "not-checked-out"), 0o755); err != nil {
		t.Fatal(err)
	}

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	err := checkDrift(filepath.Join(ws, "graycode"))
	_ = w.Close()
	os.Stdout = old
	_, _ = io.Copy(io.Discard, r)

	if err != nil {
		t.Fatalf("checkDrift with missing sibling go.mod: %v", err)
	}
}
