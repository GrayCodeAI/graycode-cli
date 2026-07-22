package lint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLanguageForExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".GO", "go"},
		{".js", "js"},
		{".jsx", "js"},
		{".mjs", "js"},
		{".ts", "ts"},
		{".tsx", "ts"},
		{".py", "python"},
		{".rs", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := LanguageForExt(tt.ext); got != tt.want {
			t.Errorf("LanguageForExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestLinterFor(t *testing.T) {
	tests := []struct {
		name    string
		lang    string
		cfg     Config
		wantNil bool
		wantStr string // substring of Name()
	}{
		{"go builtin", "go", Config{}, false, "go vet"},
		{"js builtin", "js", Config{}, false, "eslint"},
		{"python builtin", "python", Config{}, false, "ruff"},
		{"unknown", "rust", Config{}, true, ""},
		{"empty lang", "", Config{}, true, ""},
		{"custom overrides", "go", Config{Custom: map[string]string{"go": "mylint"}}, false, "custom:go"},
		{"custom empty falls back", "go", Config{Custom: map[string]string{"go": "  "}}, false, "go vet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinterFor(tt.lang, tt.cfg)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("LinterFor(%q) = %v, want nil", tt.lang, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("LinterFor(%q) = nil, want non-nil", tt.lang)
			}
			if !strings.Contains(got.Name(), tt.wantStr) {
				t.Errorf("Name() = %q, want substring %q", got.Name(), tt.wantStr)
			}
		})
	}
}

func TestParseCustomFlag(t *testing.T) {
	tests := []struct {
		in   string
		lang string
		cmd  string
		ok   bool
	}{
		{"go: golangci-lint run", "go", "golangci-lint run", true},
		{"py:ruff check {file}", "py", "ruff check {file}", true},
		{"noColon", "", "", false},
		{": cmd", "", "", false},
		{"lang:", "", "", false},
	}
	for _, tt := range tests {
		lang, cmd, ok := ParseCustomFlag(tt.in)
		if ok != tt.ok || lang != tt.lang || cmd != tt.cmd {
			t.Errorf("ParseCustomFlag(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tt.in, lang, cmd, ok, tt.lang, tt.cmd, tt.ok)
		}
	}
}

func TestRunLintDisabled(t *testing.T) {
	res := RunLint(context.Background(), "x.go", Config{Enabled: false})
	if !res.OK || res.Ran {
		t.Errorf("disabled lint should be OK no-op, got %+v", res)
	}
}

func TestRunLintUnknownLanguage(t *testing.T) {
	res := RunLint(context.Background(), "x.rs", Config{Enabled: true})
	if !res.OK || res.Ran {
		t.Errorf("unknown lang should be OK no-op, got %+v", res)
	}
}

// TestGoLintDetectsVetFailure writes a Go file that fails `go vet` (a Printf
// format mismatch) and asserts the linter surfaces the diagnostic output.
func TestGoLintDetectsVetFailure(t *testing.T) {
	if _, err := lookGo(); err != nil {
		// FIXME: test skipped in TestGoLintDetectsVetFailure
		// FIXME: go toolchain is required to run go vet linter
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	// A valid module so `go vet <file>` resolves the package.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintvettest\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main

import "fmt"

func main() {
	// %d with a string argument is a vet error.
	fmt.Printf("%d\n", "not a number")
}
`
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	res := RunLint(context.Background(), file, Config{Enabled: true})
	if !res.Ran {
		t.Fatalf("expected linter to run, got %+v", res)
	}
	if res.OK {
		t.Fatalf("expected go vet failure, got OK with output %q", res.Output)
	}
	if !strings.Contains(res.Output, "Printf") && !strings.Contains(res.Output, "%d") && !strings.Contains(strings.ToLower(res.Output), "format") {
		t.Errorf("expected vet output to mention the format issue, got: %q", res.Output)
	}
}

// TestGoLintPassesCleanFile asserts a well-formed file produces OK.
// FIXME: test skipped in main
func TestGoLintPassesCleanFile(t *testing.T) {
	if _, err := lookGo(); err != nil {
		// FIXME: go toolchain is required to run go vet linter
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintclean\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nfunc main() {}\n"
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res := RunLint(context.Background(), file, Config{Enabled: true})
	if res.Ran && !res.OK {
		t.Errorf("clean file should pass, got output: %q", res.Output)
	}
}

// TestCustomLinterFailure runs a custom linter that always exits non-zero.
func TestCustomLinterFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Enabled: true, Custom: map[string]string{"go": "echo boom; exit 1"}}
	res := RunLint(context.Background(), file, cfg)
	if !res.Ran || res.OK {
		t.Fatalf("expected custom linter failure, got %+v", res)
	}
	if !strings.Contains(res.Output, "boom") {
		t.Errorf("expected custom output to contain 'boom', got %q", res.Output)
	}
}

// TestCustomLinterFileToken verifies {file} substitution.
func TestCustomLinterFileToken(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fail and echo the path so we can confirm substitution happened.
	cfg := Config{Enabled: true, Custom: map[string]string{"go": "echo got {file}; exit 2"}}
	res := RunLint(context.Background(), file, cfg)
	if res.OK {
		t.Fatalf("expected failure, got OK")
	}
	if !strings.Contains(res.Output, file) {
		t.Errorf("expected {file} substituted to %q in output %q", file, res.Output)
	}
}

func lookGo() (string, error) {
	return exec.LookPath("go")
}
