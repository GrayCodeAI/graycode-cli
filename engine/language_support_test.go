package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNewLanguageRegistry(t *testing.T) {
	r := NewLanguageRegistry()
	if r == nil {
		t.Fatal("NewLanguageRegistry returned nil")
	}
	if len(r.Languages) < 10 {
		t.Fatalf("expected at least 10 languages, got %d", len(r.Languages))
	}
}

func TestGetByName(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name     string
		expected string
	}{
		{"Go", "Go"},
		{"go", "Go"},
		{"GO", "Go"},
		{"python", "Python"},
		{"RUST", "Rust"},
		{"typescript", "TypeScript"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		cfg := r.GetByName(tt.name)
		if tt.expected == "" {
			if cfg != nil {
				t.Errorf("GetByName(%q) = %v, want nil", tt.name, cfg.Name)
			}
		} else {
			if cfg == nil {
				t.Errorf("GetByName(%q) = nil, want %q", tt.name, tt.expected)
			} else if cfg.Name != tt.expected {
				t.Errorf("GetByName(%q).Name = %q, want %q", tt.name, cfg.Name, tt.expected)
			}
		}
	}
}

func TestGetByExtension(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		ext      string
		expected string
	}{
		{".go", "Go"},
		{".py", "Python"},
		{".ts", "TypeScript"},
		{".tsx", "TypeScript"},
		{".js", "JavaScript"},
		{".jsx", "JavaScript"},
		{".rs", "Rust"},
		{".rb", "Ruby"},
		{".java", "Java"},
		{".cs", "C#"},
		{".sh", "Shell"},
		{".bash", "Shell"},
		{".sql", "SQL"},
		{".kt", "Kotlin"},
		{".swift", "Swift"},
		{".xyz", ""},
	}

	for _, tt := range tests {
		cfg := r.GetByExtension(tt.ext)
		if tt.expected == "" {
			if cfg != nil {
				t.Errorf("GetByExtension(%q) = %v, want nil", tt.ext, cfg.Name)
			}
		} else {
			if cfg == nil {
				t.Errorf("GetByExtension(%q) = nil, want %q", tt.ext, tt.expected)
			} else if cfg.Name != tt.expected {
				t.Errorf("GetByExtension(%q).Name = %q, want %q", tt.ext, cfg.Name, tt.expected)
			}
		}
	}
}

func TestLanguageSupportRegister(t *testing.T) {
	r := NewLanguageRegistry()

	cfg := &LanguageConfig{
		Name:         "Zig",
		Extensions:   []string{".zig"},
		TestCommand:  "zig build test",
		LintCommand:  "",
		FormatCommand: "zig fmt",
		BuildCommand: "zig build",
		PackageManager: "zig",
		PackageFile:  "build.zig",
		CommentStyle: "//",
	}

	r.Register(cfg)

	got := r.GetByName("zig")
	if got == nil {
		t.Fatal("Register did not add language")
	}
	if got.Name != "Zig" {
		t.Errorf("registered language Name = %q, want %q", got.Name, "Zig")
	}
	if got.TestCommand != "zig build test" {
		t.Errorf("registered language TestCommand = %q, want %q", got.TestCommand, "zig build test")
	}

	// Verify it can be looked up by extension.
	byExt := r.GetByExtension(".zig")
	if byExt == nil || byExt.Name != "Zig" {
		t.Error("registered language not found by extension")
	}
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewLanguageRegistry()

	// Overwrite Go with a custom config.
	cfg := &LanguageConfig{
		Name:        "Go",
		Extensions:  []string{".go"},
		TestCommand: "go test -v ./...",
		CommentStyle: "//",
	}

	r.Register(cfg)

	got := r.GetByName("go")
	if got == nil {
		t.Fatal("overwritten language not found")
	}
	if got.TestCommand != "go test -v ./..." {
		t.Errorf("overwritten TestCommand = %q, want %q", got.TestCommand, "go test -v ./...")
	}
}

func TestDetect(t *testing.T) {
	// Create a temporary project with Go files.
	dir := t.TempDir()

	// Create go.mod.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create some .go files.
	for _, name := range []string{"main.go", "util.go", "handler.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	r := NewLanguageRegistry()
	cfg := r.Detect(dir)
	if cfg == nil {
		t.Fatal("Detect returned nil for Go project")
	}
	if cfg.Name != "Go" {
		t.Errorf("Detect = %q, want %q", cfg.Name, "Go")
	}
}

func TestDetectAll(t *testing.T) {
	dir := t.TempDir()

	// Create a mixed project: Go primary, some Shell scripts, a SQL file.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.go", "handler.go", "util.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "deploy.sh"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "setup.sh"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.sql"), []byte("CREATE TABLE t (id INT);\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewLanguageRegistry()
	configs := r.DetectAll(dir)

	if len(configs) < 3 {
		t.Fatalf("DetectAll returned %d configs, want at least 3", len(configs))
	}

	// First should be Go (most files + package file boost).
	if configs[0].Name != "Go" {
		t.Errorf("primary language = %q, want Go", configs[0].Name)
	}

	// Verify Shell and SQL are present.
	names := make(map[string]bool)
	for _, c := range configs {
		names[c.Name] = true
	}
	if !names["Shell"] {
		t.Error("Shell not detected")
	}
	if !names["SQL"] {
		t.Error("SQL not detected")
	}
}

func TestDetectEmptyDir(t *testing.T) {
	dir := t.TempDir()

	r := NewLanguageRegistry()
	cfg := r.Detect(dir)
	if cfg != nil {
		t.Errorf("Detect on empty dir = %v, want nil", cfg.Name)
	}

	configs := r.DetectAll(dir)
	if configs != nil {
		t.Errorf("DetectAll on empty dir = %v, want nil", configs)
	}
}

func TestDetectSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	// Put Go files only in a hidden dir.
	hidden := filepath.Join(dir, ".hidden")
	if err := os.Mkdir(hidden, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Put a Python file at root.
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('hi')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewLanguageRegistry()
	cfg := r.Detect(dir)
	if cfg == nil {
		t.Fatal("Detect returned nil")
	}
	// Should detect Python, not Go (hidden dir skipped).
	if cfg.Name != "Python" {
		t.Errorf("Detect = %q, want Python (hidden dir should be skipped)", cfg.Name)
	}
}

func TestTestCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewLanguageRegistry()
	cmd := r.TestCommand(dir)
	if cmd != "go test ./..." {
		t.Errorf("TestCommand = %q, want %q", cmd, "go test ./...")
	}
}

func TestLintCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.rs"), []byte("fn main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewLanguageRegistry()
	cmd := r.LintCommand(dir)
	if cmd != "cargo clippy" {
		t.Errorf("LintCommand = %q, want %q", cmd, "cargo clippy")
	}
}

func TestFormatCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('hi')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewLanguageRegistry()
	cmd := r.FormatCommand(dir)
	if cmd != "black" {
		t.Errorf("FormatCommand = %q, want %q", cmd, "black")
	}
}

func TestCommandsEmptyDir(t *testing.T) {
	dir := t.TempDir()

	r := NewLanguageRegistry()
	if cmd := r.TestCommand(dir); cmd != "" {
		t.Errorf("TestCommand on empty dir = %q, want empty", cmd)
	}
	if cmd := r.LintCommand(dir); cmd != "" {
		t.Errorf("LintCommand on empty dir = %q, want empty", cmd)
	}
	if cmd := r.FormatCommand(dir); cmd != "" {
		t.Errorf("FormatCommand on empty dir = %q, want empty", cmd)
	}
}

func TestFormatLanguages(t *testing.T) {
	r := NewLanguageRegistry()

	configs := []*LanguageConfig{
		r.GetByName("Go"),
		r.GetByName("Shell"),
		r.GetByName("SQL"),
	}

	output := FormatLanguages(configs)

	if !strings.Contains(output, "Detected Languages:") {
		t.Error("output missing header")
	}
	if !strings.Contains(output, "─") {
		t.Error("output missing separator")
	}
	if !strings.Contains(output, "Go (primary)") {
		t.Error("output missing primary marker")
	}
	if !strings.Contains(output, "test=go") {
		t.Errorf("output missing Go test command, got:\n%s", output)
	}
	if !strings.Contains(output, "lint=golangci-lint") {
		t.Errorf("output missing Go lint command, got:\n%s", output)
	}
	if !strings.Contains(output, "fmt=gofmt") {
		t.Errorf("output missing Go format command, got:\n%s", output)
	}
	if !strings.Contains(output, "Shell:") {
		t.Error("output missing Shell")
	}
	if !strings.Contains(output, "SQL:") {
		t.Error("output missing SQL")
	}
	// SQL has no test or format, only lint.
	if !strings.Contains(output, "lint=sqlfluff") {
		t.Errorf("output missing SQL lint, got:\n%s", output)
	}
}

func TestFormatLanguagesEmpty(t *testing.T) {
	output := FormatLanguages(nil)
	if output != "No languages detected." {
		t.Errorf("FormatLanguages(nil) = %q, want %q", output, "No languages detected.")
	}
}

func TestLanguageConfigFields(t *testing.T) {
	r := NewLanguageRegistry()

	// Verify specific language configurations.
	goLang := r.GetByName("Go")
	if goLang == nil {
		t.Fatal("Go language not found")
	}
	if goLang.PackageManager != "go" {
		t.Errorf("Go PackageManager = %q, want %q", goLang.PackageManager, "go")
	}
	if goLang.PackageFile != "go.mod" {
		t.Errorf("Go PackageFile = %q, want %q", goLang.PackageFile, "go.mod")
	}
	if goLang.CommentStyle != "//" {
		t.Errorf("Go CommentStyle = %q, want %q", goLang.CommentStyle, "//")
	}
	if goLang.ImportPattern == nil {
		t.Error("Go ImportPattern is nil")
	}
	if goLang.FunctionPattern == nil {
		t.Error("Go FunctionPattern is nil")
	}

	python := r.GetByName("Python")
	if python == nil {
		t.Fatal("Python language not found")
	}
	if python.CommentStyle != "#" {
		t.Errorf("Python CommentStyle = %q, want %q", python.CommentStyle, "#")
	}
	if python.BuildCommand != "" {
		t.Errorf("Python BuildCommand = %q, want empty", python.BuildCommand)
	}

	sql := r.GetByName("SQL")
	if sql == nil {
		t.Fatal("SQL language not found")
	}
	if sql.CommentStyle != "--" {
		t.Errorf("SQL CommentStyle = %q, want %q", sql.CommentStyle, "--")
	}
}

func TestImportPatterns(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		lang    string
		input   string
		matches bool
	}{
		{"Go", `	"fmt"`, true},
		{"Go", `	"github.com/foo/bar"`, true},
		{"Python", `import os`, true},
		{"Python", `from flask import Flask`, true},
		{"Rust", `use std::io;`, true},
		{"Ruby", `require 'json'`, true},
		{"TypeScript", `import { Foo } from './bar'`, true},
		{"Java", `import java.util.List;`, true},
		{"Shell", `source ./lib.sh`, true},
	}

	for _, tt := range tests {
		cfg := r.GetByName(tt.lang)
		if cfg == nil {
			t.Fatalf("language %q not found", tt.lang)
		}
		if cfg.ImportPattern == nil {
			t.Fatalf("language %q has nil ImportPattern", tt.lang)
		}
		got := cfg.ImportPattern.MatchString(tt.input)
		if got != tt.matches {
			t.Errorf("%s ImportPattern.Match(%q) = %v, want %v", tt.lang, tt.input, got, tt.matches)
		}
	}
}

func TestFunctionPatterns(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		lang     string
		input    string
		matches  bool
		funcName string
	}{
		{"Go", "func main()", true, "main"},
		{"Go", "func HandleRequest(w http.ResponseWriter)", true, "HandleRequest"},
		{"Python", "def process_data(x):", true, "process_data"},
		{"Rust", "fn calculate(x: i32) -> i32 {", true, "calculate"},
		{"Rust", "pub fn new() -> Self {", true, "new"},
		{"Ruby", "def initialize", true, "initialize"},
		{"Shell", "deploy() {", true, "deploy"},
		{"Shell", "function cleanup() {", true, "cleanup"},
	}

	for _, tt := range tests {
		cfg := r.GetByName(tt.lang)
		if cfg == nil {
			t.Fatalf("language %q not found", tt.lang)
		}
		if cfg.FunctionPattern == nil {
			t.Fatalf("language %q has nil FunctionPattern", tt.lang)
		}
		matches := cfg.FunctionPattern.FindStringSubmatch(tt.input)
		got := len(matches) > 0
		if got != tt.matches {
			t.Errorf("%s FunctionPattern.Match(%q) = %v, want %v", tt.lang, tt.input, got, tt.matches)
			continue
		}
		if got && len(matches) > 1 && matches[1] != tt.funcName {
			t.Errorf("%s FunctionPattern(%q) captured %q, want %q", tt.lang, tt.input, matches[1], tt.funcName)
		}
	}
}

func TestLanguageSupportConcurrentAccess(t *testing.T) {
	r := NewLanguageRegistry()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.GetByName("go")
			r.GetByExtension(".py")
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		r.Register(&LanguageConfig{
			Name:       "CustomLang",
			Extensions: []string{".custom"},
		})
	}

	<-done
}

func TestDetectWithNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Create node_modules with many .js files (should be skipped).
	nm := filepath.Join(dir, "node_modules", "some-pkg")
	if err := os.MkdirAll(nm, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		name := filepath.Join(nm, strings.Repeat("a", i+1)+".js")
		if err := os.WriteFile(name, []byte("//"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a single .ts file at root.
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.ts"), []byte("export {}"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewLanguageRegistry()
	cfg := r.Detect(dir)
	if cfg == nil {
		t.Fatal("Detect returned nil")
	}
	// Should detect TypeScript (node_modules skipped).
	if cfg.Name != "TypeScript" {
		t.Errorf("Detect = %q, want TypeScript (node_modules should be skipped)", cfg.Name)
	}
}

func TestShortCmd(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"go test ./...", "go"},
		{"golangci-lint run", "golangci-lint"},
		{"./gradlew test", "gradlew"},
		{"cargo clippy", "cargo"},
		{"prettier --write", "prettier"},
		{"bundle exec rspec", "bundle"},
		{"dotnet format --verify-no-changes", "dotnet"},
	}

	for _, tt := range tests {
		got := shortCmd(tt.input)
		if got != tt.expected {
			t.Errorf("shortCmd(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDetectPythonProject(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app.py", "models.py", "views.py"} {
		if err := os.WriteFile(filepath.Join(dir, "src", name), []byte("# python\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	r := NewLanguageRegistry()
	cfg := r.Detect(dir)
	if cfg == nil {
		t.Fatal("Detect returned nil")
	}
	if cfg.Name != "Python" {
		t.Errorf("Detect = %q, want Python", cfg.Name)
	}

	cmd := r.TestCommand(dir)
	if cmd != "pytest" {
		t.Errorf("TestCommand = %q, want %q", cmd, "pytest")
	}
}

func TestGetByExtensionCaseInsensitive(t *testing.T) {
	r := NewLanguageRegistry()

	// Extension should be matched case-insensitively.
	cfg := r.GetByExtension(".GO")
	if cfg == nil {
		t.Fatal("GetByExtension(.GO) returned nil")
	}
	if cfg.Name != "Go" {
		t.Errorf("GetByExtension(.GO) = %q, want Go", cfg.Name)
	}
}

func TestSQLHasNoTestCommand(t *testing.T) {
	r := NewLanguageRegistry()
	sql := r.GetByName("SQL")
	if sql == nil {
		t.Fatal("SQL not found")
	}
	if sql.TestCommand != "" {
		t.Errorf("SQL TestCommand = %q, want empty", sql.TestCommand)
	}
	if sql.LintCommand != "sqlfluff lint" {
		t.Errorf("SQL LintCommand = %q, want %q", sql.LintCommand, "sqlfluff lint")
	}
}

func TestAllLanguagesHaveCommentStyle(t *testing.T) {
	r := NewLanguageRegistry()
	for name, cfg := range r.Languages {
		if cfg.CommentStyle == "" {
			t.Errorf("language %q has empty CommentStyle", name)
		}
	}
}

func TestAllLanguagesHaveExtensions(t *testing.T) {
	r := NewLanguageRegistry()
	for name, cfg := range r.Languages {
		if len(cfg.Extensions) == 0 {
			t.Errorf("language %q has no extensions", name)
		}
	}
}

func TestImportPatternIsRegexp(t *testing.T) {
	r := NewLanguageRegistry()
	for name, cfg := range r.Languages {
		// SQL may legitimately have nil ImportPattern.
		if cfg.ImportPattern == nil && name != "sql" {
			t.Errorf("language %q has nil ImportPattern", name)
		}
		if cfg.ImportPattern != nil {
			// Verify it's a valid compiled regex by calling a method.
			_ = cfg.ImportPattern.MatchString("")
		}
	}
}

func TestFunctionPatternCapture(t *testing.T) {
	// Verify all function patterns have at least one capture group.
	r := NewLanguageRegistry()
	for name, cfg := range r.Languages {
		if cfg.FunctionPattern == nil {
			t.Errorf("language %q has nil FunctionPattern", name)
			continue
		}
		if cfg.FunctionPattern.NumSubexp() < 1 {
			t.Errorf("language %q FunctionPattern has no capture groups", name)
		}
	}
}

func TestRegisterCustomLanguage(t *testing.T) {
	r := NewLanguageRegistry()

	r.Register(&LanguageConfig{
		Name:            "HCL",
		Extensions:      []string{".tf", ".hcl"},
		TestCommand:     "terraform validate",
		LintCommand:     "tflint",
		FormatCommand:   "terraform fmt",
		BuildCommand:    "terraform plan",
		PackageManager:  "",
		PackageFile:     "",
		ImportPattern:   regexp.MustCompile(`^\s*module\s+"([^"]+)"`),
		FunctionPattern: regexp.MustCompile(`^\s*(?:resource|data)\s+"(\w+)"`),
		CommentStyle:    "#",
	})

	cfg := r.GetByExtension(".tf")
	if cfg == nil {
		t.Fatal("custom .tf extension not found")
	}
	if cfg.Name != "HCL" {
		t.Errorf("custom language Name = %q, want HCL", cfg.Name)
	}
	if cfg.LintCommand != "tflint" {
		t.Errorf("custom language LintCommand = %q, want tflint", cfg.LintCommand)
	}
}
