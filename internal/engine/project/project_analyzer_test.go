package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProjectAnalyzer(t *testing.T) {
	pa := NewProjectAnalyzer("/tmp/test-dir")
	if pa == nil {
		t.Fatal("NewProjectAnalyzer returned nil")
	}
	if pa.Dir != "/tmp/test-dir" {
		t.Errorf("expected Dir=/tmp/test-dir, got %s", pa.Dir)
	}
}

func TestDetectArchitecture(t *testing.T) {
	tests := []struct {
		name     string
		dirs     []string
		files    map[string]string
		expected string
	}{
		{
			name:     "hexagonal",
			dirs:     []string{"domain", "ports", "adapters"},
			expected: "hexagonal",
		},
		{
			name:     "layered with cmd and engine",
			dirs:     []string{"cmd", "engine", "tool"},
			files:    map[string]string{"cmd/main.go": "package main", "engine/e.go": "package engine", "tool/t.go": "package tool"},
			expected: "layered",
		},
		{
			name:     "microservices",
			dirs:     []string{"auth-service", "user-service", "api-gateway"},
			expected: "microservices",
		},
		{
			name:     "monolith",
			dirs:     []string{},
			files:    map[string]string{"main.go": "package main\nfunc main() {}"},
			expected: "monolith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			for _, d := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			for path, content := range tt.files {
				fullPath := filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			result := DetectArchitecture(dir)
			if result != tt.expected {
				t.Errorf("DetectArchitecture() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestProjectAnalyzerDetectPatterns(t *testing.T) {
	dir := t.TempDir()

	// Create files with repository pattern.
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repoContent := `package repo

type UserRepository interface {
	FindByID(id string) (*User, error)
	Save(u *User) error
}

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}
`
	if err := os.WriteFile(filepath.Join(repoDir, "user_repository.go"), []byte(repoContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create files with middleware pattern.
	mwDir := filepath.Join(dir, "middleware")
	if err := os.MkdirAll(mwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mwContent := `package middleware

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
`
	if err := os.WriteFile(filepath.Join(mwDir, "auth_middleware.go"), []byte(mwContent), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := DetectPatterns(dir)

	// Should detect at least repository and middleware patterns.
	foundRepo := false
	foundMiddleware := false
	for _, p := range patterns {
		if p.Name == "Repository" {
			foundRepo = true
			if p.Confidence < 0.5 {
				t.Errorf("Repository pattern confidence too low: %f", p.Confidence)
			}
		}
		if p.Name == "Middleware" {
			foundMiddleware = true
		}
	}

	if !foundRepo {
		t.Error("expected Repository pattern to be detected")
	}
	if !foundMiddleware {
		t.Error("expected Middleware pattern to be detected")
	}
}

func TestDetectPatternsFactory(t *testing.T) {
	dir := t.TempDir()

	// Create file with multiple New* constructors.
	factoryContent := `package factory

type Service struct{}
type Client struct{}
type Worker struct{}

func NewService() *Service { return &Service{} }
func NewClient() *Client { return &Client{} }
func NewWorker() *Worker { return &Worker{} }
`
	if err := os.WriteFile(filepath.Join(dir, "factory.go"), []byte(factoryContent), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := DetectPatterns(dir)

	foundFactory := false
	for _, p := range patterns {
		if p.Name == "Factory" {
			foundFactory = true
			break
		}
	}
	if !foundFactory {
		t.Error("expected Factory pattern to be detected")
	}
}

func TestDetectPatternsFunctionalOptions(t *testing.T) {
	dir := t.TempDir()

	optContent := `package config

type Option func(*Config)

func WithTimeout(d int) Option { return func(c *Config) { c.Timeout = d } }
func WithRetries(n int) Option { return func(c *Config) { c.Retries = n } }
func WithName(s string) Option { return func(c *Config) { c.Name = s } }
func WithDebug(b bool) Option  { return func(c *Config) { c.Debug = b } }
`
	if err := os.WriteFile(filepath.Join(dir, "options.go"), []byte(optContent), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := DetectPatterns(dir)

	found := false
	for _, p := range patterns {
		if p.Name == "Functional Options" {
			found = true
			if p.Confidence < 0.5 {
				t.Errorf("Functional Options confidence too low: %f", p.Confidence)
			}
			break
		}
	}
	if !found {
		t.Error("expected Functional Options pattern to be detected")
	}
}

func TestAnalyzeModule(t *testing.T) {
	dir := t.TempDir()

	content := `package mymod

// MyStruct is a public struct.
type MyStruct struct {
	Name string
}

// NewMyStruct creates a new instance.
func NewMyStruct(name string) *MyStruct {
	return &MyStruct{Name: name}
}

// DoSomething does something publicly.
func (m *MyStruct) DoSomething() error {
	return nil
}

// private is not exported.
func private() {}
`
	if err := os.WriteFile(filepath.Join(dir, "mymod.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pa := NewProjectAnalyzer(filepath.Dir(dir))
	info := pa.AnalyzeModule(dir)

	if info.Name != filepath.Base(dir) {
		t.Errorf("expected Name=%s, got %s", filepath.Base(dir), info.Name)
	}

	if info.Size == 0 {
		t.Error("expected Size > 0")
	}

	// Check public API includes exported items.
	hasMyStruct := false
	hasNewMyStruct := false
	hasDoSomething := false
	for _, api := range info.PublicAPI {
		if api == "MyStruct" {
			hasMyStruct = true
		}
		if api == "NewMyStruct" {
			hasNewMyStruct = true
		}
		if strings.Contains(api, "DoSomething") {
			hasDoSomething = true
		}
	}

	if !hasMyStruct {
		t.Error("expected MyStruct in PublicAPI")
	}
	if !hasNewMyStruct {
		t.Error("expected NewMyStruct in PublicAPI")
	}
	if !hasDoSomething {
		t.Error("expected DoSomething in PublicAPI")
	}
}

func TestAnalyzeNonExistentDir(t *testing.T) {
	pa := NewProjectAnalyzer("/nonexistent/path/xyz")
	_, err := pa.Analyze()
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestAnalyzeEmptyProject(t *testing.T) {
	dir := t.TempDir()

	pa := NewProjectAnalyzer(dir)
	analysis, err := pa.Analyze()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Name == "" {
		t.Error("expected Name to be set from directory name")
	}
	if analysis.LOC != 0 {
		t.Errorf("expected LOC=0 for empty project, got %d", analysis.LOC)
	}
}

func TestAnalyzeFullProject(t *testing.T) {
	dir := t.TempDir()

	// Create go.mod.
	goMod := `module github.com/example/testproject

go 1.21

require (
	github.com/spf13/cobra v1.7.0
	github.com/stretchr/testify v1.8.4
)
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create cmd directory.
	cmdDir := filepath.Join(dir, "cmd", "myapp")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainContent := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create engine package.
	engineDir := filepath.Join(dir, "engine")
	if err := os.MkdirAll(engineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	engineContent := `package engine

import "context"

// Engine drives the core loop.
type Engine struct {
	running bool
}

// NewEngine creates a new Engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Run starts the engine with context.
func (e *Engine) Run(ctx context.Context) error {
	e.running = true
	return nil
}
`
	if err := os.WriteFile(filepath.Join(engineDir, "engine.go"), []byte(engineContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create test file.
	testContent := `package engine

import "testing"

func TestEngine(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"basic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}
`
	if err := os.WriteFile(filepath.Join(engineDir, "engine_test.go"), []byte(testContent), 0o644); err != nil {
		t.Fatal(err)
	}

	pa := NewProjectAnalyzer(dir)
	analysis, err := pa.Analyze()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check basic fields.
	if analysis.Name != "testproject" {
		t.Errorf("expected Name=testproject, got %s", analysis.Name)
	}
	if analysis.Language != "Go" {
		t.Errorf("expected Language=Go, got %s", analysis.Language)
	}
	if analysis.Architecture != "layered" {
		t.Errorf("expected Architecture=layered, got %s", analysis.Architecture)
	}
	if analysis.Dependencies != 2 {
		t.Errorf("expected Dependencies=2, got %d", analysis.Dependencies)
	}
	if analysis.LOC == 0 {
		t.Error("expected LOC > 0")
	}

	// Check entry points.
	foundEntry := false
	for _, ep := range analysis.EntryPoints {
		if strings.Contains(ep, "main.go") {
			foundEntry = true
		}
	}
	if !foundEntry {
		t.Error("expected main.go in entry points")
	}

	// Check test coverage is not empty.
	if analysis.TestCoverage == "" || analysis.TestCoverage == "unknown" {
		t.Error("expected non-empty test coverage")
	}
}

func TestGenerateOnboardingDoc(t *testing.T) {
	analysis := &ProjectAnalysis{
		Name:         "hawk",
		Language:     "Go",
		Framework:    "Cobra CLI + Bubbletea TUI",
		Architecture: "layered",
		EntryPoints:  []string{"cmd/hawk/main.go"},
		KeyModules: []ModuleInfo{
			{Name: "engine", Path: "engine", Purpose: "Agent loop, compaction, streaming", Size: 14000},
			{Name: "tool", Path: "tool", Purpose: "40 built-in tools", Size: 8000},
			{Name: "config", Path: "config", Purpose: "Settings, validation", Size: 3000},
		},
		Patterns: []Pattern{
			{Name: "Interface-driven tools", Description: "Tool interface", Confidence: 0.9},
			{Name: "Functional Options", Description: "WithXxx pattern", Confidence: 0.85},
			{Name: "Observer", Description: "EventBus", Confidence: 0.75},
		},
		Conventions:  []string{"Error wrapping with %w", "Table-driven tests", "Cobra for CLI commands"},
		Dependencies: 25,
		TestCoverage: "85% (17/20 packages have tests)",
		LOC:          45000,
		Complexity:   "moderate (120/800 functions >50 lines)",
	}

	doc := GenerateOnboardingDoc(analysis)

	// Verify key sections exist.
	expectedSections := []string{
		"# Project: hawk",
		"## Architecture: Layered",
		"## Key Modules",
		"engine (14K LOC)",
		"tool (8K LOC)",
		"## Patterns Detected",
		"Interface-driven tools",
		"Functional Options",
		"## Conventions",
		"Error wrapping with %w",
		"Table-driven tests",
		"## Stats",
		"Language: Go",
	}

	for _, expected := range expectedSections {
		if !strings.Contains(doc, expected) {
			t.Errorf("onboarding doc missing expected section: %q", expected)
		}
	}
}

func TestFormatAnalysis(t *testing.T) {
	analysis := &ProjectAnalysis{
		Name:         "myproject",
		Language:     "Go",
		Framework:    "Gin",
		Architecture: "layered",
		EntryPoints:  []string{"cmd/main.go"},
		KeyModules:   []ModuleInfo{{Name: "api", Size: 2000}},
		Patterns:     []Pattern{{Name: "Repository", Confidence: 0.8}},
		Dependencies: 10,
		TestCoverage: "75%",
		LOC:          5000,
		Complexity:   "low",
	}

	result := FormatAnalysis(analysis)

	if !strings.Contains(result, "myproject") {
		t.Error("expected project name in format output")
	}
	if !strings.Contains(result, "Go / Gin") {
		t.Error("expected language/framework in format output")
	}
	if !strings.Contains(result, "layered") {
		t.Error("expected architecture in format output")
	}
	if !strings.Contains(result, "Repository") {
		t.Error("expected pattern name in format output")
	}
}

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		threshold int
		minConf   float64
		maxConf   float64
	}{
		{"zero files", 0, 3, 0.0, 0.01},
		{"below threshold", 1, 3, 0.5, 0.85},
		{"at threshold", 3, 3, 0.79, 0.81},
		{"double threshold", 6, 3, 0.94, 0.96},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := make([]string, tt.count)
			for i := range files {
				files[i] = "file.go"
			}
			conf := calculateConfidence(files, tt.threshold)
			if conf < tt.minConf || conf > tt.maxConf {
				t.Errorf("calculateConfidence(%d, %d) = %f, want [%f, %f]",
					tt.count, tt.threshold, conf, tt.minConf, tt.maxConf)
			}
		})
	}
}

func TestCountFileLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lines := countFileLines(path)
	if lines != 5 {
		t.Errorf("expected 5 lines, got %d", lines)
	}
}

func TestCountFileLinesNonExistent(t *testing.T) {
	lines := countFileLines("/nonexistent/file.go")
	if lines != 0 {
		t.Errorf("expected 0 lines for non-existent file, got %d", lines)
	}
}

func TestFormatLOC(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500 LOC"},
		{1000, "1K LOC"},
		{14500, "14K LOC"},
		{0, "0 LOC"},
	}

	for _, tt := range tests {
		result := formatLOC(tt.input)
		if result != tt.expected {
			t.Errorf("formatLOC(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestHasGoFiles(t *testing.T) {
	dir := t.TempDir()

	// Empty directory should have no go files.
	if hasGoFiles(dir) {
		t.Error("expected no go files in empty dir")
	}

	// Add a go file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !hasGoFiles(dir) {
		t.Error("expected go files after adding main.go")
	}
}

func TestHasMainPackage(t *testing.T) {
	dir := t.TempDir()

	if hasMainPackage(dir) {
		t.Error("expected no main package in empty dir")
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !hasMainPackage(dir) {
		t.Error("expected main package after adding main.go")
	}
}

func TestProjAnalyzerExprToString(t *testing.T) {
	// Verify the function exists and handles nil gracefully.
	result := projAnalyzerExprToString(nil)
	if result != "T" {
		t.Errorf("projAnalyzerExprToString(nil) = %q, want 'T'", result)
	}
}

func TestProjAnalyzerAppendUnique(t *testing.T) {
	slice := []string{"a", "b", "c"}

	// Adding existing element should not change slice.
	result := projAnalyzerAppendUnique(slice, "b")
	if len(result) != 3 {
		t.Errorf("expected length 3, got %d", len(result))
	}

	// Adding new element should append.
	result = projAnalyzerAppendUnique(slice, "d")
	if len(result) != 4 {
		t.Errorf("expected length 4, got %d", len(result))
	}
}

func TestDetectArchitectureNonExistent(t *testing.T) {
	result := DetectArchitecture("/nonexistent/path/xyz")
	if result != "unknown" {
		t.Errorf("expected 'unknown' for non-existent dir, got %q", result)
	}
}

func TestModularArchitecture(t *testing.T) {
	dir := t.TempDir()

	// Create 6 feature directories with go files (no cmd).
	features := []string{"auth", "users", "orders", "payments", "notifications", "analytics"}
	for _, f := range features {
		fDir := filepath.Join(dir, f)
		if err := os.MkdirAll(fDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fDir, f+".go"), []byte("package "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := DetectArchitecture(dir)
	if result != "modular" {
		t.Errorf("expected 'modular' architecture, got %q", result)
	}
}
