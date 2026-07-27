package lint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestEslintLinter_NoTool tests eslint linter when npx/eslint are not available.
func TestEslintLinter_NoTool(t *testing.T) {
	// Use a restricted PATH that doesn't have npx or eslint
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	linter := eslintLinter{}
	if linter.Name() != "eslint" {
		t.Errorf("expected name 'eslint', got %q", linter.Name())
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "test.js")
	if err := os.WriteFile(file, []byte("const x = 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	// Without npx/eslint, should return OK=true (no-op)
	if !res.OK {
		t.Errorf("expected OK=true when eslint not available, got %+v", res)
	}
	if res.Ran {
		t.Error("expected Ran=false when eslint not available")
	}
}

// TestRuffLinter_NoTool tests ruff linter when ruff is not available.
func TestRuffLinter_NoTool(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	linter := ruffLinter{}
	if linter.Name() != "ruff" {
		t.Errorf("expected name 'ruff', got %q", linter.Name())
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "test.py")
	if err := os.WriteFile(file, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	// Without ruff, should return OK=true (no-op)
	if !res.OK {
		t.Errorf("expected OK=true when ruff not available, got %+v", res)
	}
	if res.Ran {
		t.Error("expected Ran=false when ruff not available")
	}
}

// TestGoLinter_NoTool tests go linter when go/gofmt are not available.
func TestGoLinter_NoTool(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	linter := goLinter{}
	if linter.Name() != "go vet/gofmt" {
		t.Errorf("expected name 'go vet/gofmt', got %q", linter.Name())
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	// Without go/gofmt, should return OK=true (no-op)
	if !res.OK {
		t.Errorf("expected OK=true when go not available, got %+v", res)
	}
}

// TestCustomLinter_Success tests a custom linter that succeeds.
func TestCustomLinter_Success(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Enabled: true, Custom: map[string]string{"go": "echo all good {file}"}}
	res := RunLint(context.Background(), file, cfg)
	if !res.Ran || !res.OK {
		t.Fatalf("expected custom linter success, got %+v", res)
	}
	if res.Output != "all good "+file {
		t.Errorf("expected output 'all good <file>', got %q", res.Output)
	}
}

// TestCustomLinter_EmptyOutput tests a custom linter that fails with no output.
func TestCustomLinter_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Enabled: true, Custom: map[string]string{"go": "exit 1"}}
	res := RunLint(context.Background(), file, cfg)
	if !res.Ran || res.OK {
		t.Fatalf("expected custom linter failure, got %+v", res)
	}
	// Output should contain the error message since stdout was empty
	if res.Output == "" {
		t.Error("expected non-empty output (error message)")
	}
}

// TestRunLint_WithTimeout tests RunLint with a custom timeout.
func TestRunLint_WithTimeout(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Enabled: true,
		Custom:  map[string]string{"go": "echo done"},
		Timeout: 5000000000, // 5s
	}
	res := RunLint(context.Background(), file, cfg)
	if !res.OK {
		t.Errorf("expected OK, got %+v", res)
	}
}

// TestLanguageForFile tests LanguageForFile.
func TestLanguageForFile(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"main.go", "go"},
		{"app.js", "js"},
		{"app.tsx", "ts"},
		{"script.py", "python"},
		{"readme.md", ""},
		{"noext", ""},
	}
	for _, tt := range tests {
		if got := LanguageForFile(tt.file); got != tt.want {
			t.Errorf("LanguageForFile(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

// TestRunLint_LanguageSet tests that RunLint sets the Language field.
func TestRunLint_LanguageSet(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.py")
	if err := os.WriteFile(file, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// With ruff not available, should still set Language
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	cfg := Config{Enabled: true}
	res := RunLint(context.Background(), file, cfg)
	if res.Language != "python" {
		t.Errorf("expected language 'python', got %q", res.Language)
	}
}

// TestCustomLinter_Name tests the custom linter Name method.
func TestCustomLinter_Name(t *testing.T) {
	linter := &customLinter{lang: "ruby", command: "rubocop"}
	if linter.Name() != "custom:ruby" {
		t.Errorf("expected 'custom:ruby', got %q", linter.Name())
	}
}

// TestEslintLinter_WithMockNpx tests eslint linter with a mock npx that succeeds.
func TestEslintLinter_WithMockNpx(t *testing.T) {
	// Create a mock npx that exits 0 with no output
	mockDir := t.TempDir()
	mockNpx := filepath.Join(mockDir, "npx")
	if err := os.WriteFile(mockNpx, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", mockDir)
	defer os.Setenv("PATH", origPath)

	linter := eslintLinter{}
	dir := t.TempDir()
	file := filepath.Join(dir, "test.js")
	if err := os.WriteFile(file, []byte("const x = 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	if !res.OK || !res.Ran {
		t.Errorf("expected OK=true Ran=true with mock npx, got %+v", res)
	}
}

// TestEslintLinter_WithMockNpx_Failure tests eslint linter with a mock npx that fails.
func TestEslintLinter_WithMockNpx_Failure(t *testing.T) {
	mockDir := t.TempDir()
	mockNpx := filepath.Join(mockDir, "npx")
	if err := os.WriteFile(mockNpx, []byte("#!/bin/sh\necho 'error: semicolon missing'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", mockDir)
	defer os.Setenv("PATH", origPath)

	linter := eslintLinter{}
	dir := t.TempDir()
	file := filepath.Join(dir, "test.js")
	if err := os.WriteFile(file, []byte("const x = 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	if res.OK {
		t.Error("expected OK=false with failing mock npx")
	}
	if !res.Ran {
		t.Error("expected Ran=true")
	}
}

// TestEslintLinter_WithMockNpx_EmptyOutput tests eslint with npx that fails silently.
func TestEslintLinter_WithMockNpx_EmptyOutput(t *testing.T) {
	mockDir := t.TempDir()
	mockNpx := filepath.Join(mockDir, "npx")
	// npx --no-install miss: exits non-zero with no output
	if err := os.WriteFile(mockNpx, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", mockDir)
	defer os.Setenv("PATH", origPath)

	linter := eslintLinter{}
	dir := t.TempDir()
	file := filepath.Join(dir, "test.js")
	if err := os.WriteFile(file, []byte("const x = 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	// Empty output + error = npx miss, treated as no-op
	if !res.OK {
		t.Errorf("expected OK=true for npx miss, got %+v", res)
	}
}

// TestEslintLinter_WithMockEslint tests eslint linter with a direct eslint binary.
func TestEslintLinter_WithMockEslint(t *testing.T) {
	mockDir := t.TempDir()
	mockEslint := filepath.Join(mockDir, "eslint")
	if err := os.WriteFile(mockEslint, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", mockDir)
	defer os.Setenv("PATH", origPath)

	linter := eslintLinter{}
	dir := t.TempDir()
	file := filepath.Join(dir, "test.js")
	if err := os.WriteFile(file, []byte("const x = 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	if !res.OK || !res.Ran {
		t.Errorf("expected OK=true Ran=true with mock eslint, got %+v", res)
	}
}

// TestRuffLinter_WithMockRuff tests ruff linter with a mock ruff that succeeds.
func TestRuffLinter_WithMockRuff(t *testing.T) {
	mockDir := t.TempDir()
	mockRuff := filepath.Join(mockDir, "ruff")
	if err := os.WriteFile(mockRuff, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", mockDir)
	defer os.Setenv("PATH", origPath)

	linter := ruffLinter{}
	dir := t.TempDir()
	file := filepath.Join(dir, "test.py")
	if err := os.WriteFile(file, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	if !res.OK || !res.Ran {
		t.Errorf("expected OK=true Ran=true with mock ruff, got %+v", res)
	}
}

// TestRuffLinter_WithMockRuff_Failure tests ruff linter with a mock ruff that fails.
func TestRuffLinter_WithMockRuff_Failure(t *testing.T) {
	mockDir := t.TempDir()
	mockRuff := filepath.Join(mockDir, "ruff")
	if err := os.WriteFile(mockRuff, []byte("#!/bin/sh\necho 'E501 line too long'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", mockDir)
	defer os.Setenv("PATH", origPath)

	linter := ruffLinter{}
	dir := t.TempDir()
	file := filepath.Join(dir, "test.py")
	if err := os.WriteFile(file, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := linter.Lint(context.Background(), file)
	if res.OK {
		t.Error("expected OK=false with failing mock ruff")
	}
	if !res.Ran {
		t.Error("expected Ran=true")
	}
}
