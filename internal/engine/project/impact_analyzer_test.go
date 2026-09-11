package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupImpactTestProject creates a temporary Go project structure for testing.
func setupImpactTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create go.mod.
	impactWriteFile(t, filepath.Join(dir, "go.mod"), "module example.com/testproj\n\ngo 1.21\n")

	// Create package structure:
	// pkg/auth/      - core auth package
	// pkg/handler/   - imports auth
	// pkg/server/    - imports handler
	// pkg/api/       - imports auth
	// cmd/app/       - imports server and api

	// pkg/auth/auth.go
	impactMkdirAll(t, filepath.Join(dir, "pkg", "auth"))
	impactWriteFile(t, filepath.Join(dir, "pkg", "auth", "auth.go"), `package auth

import "fmt"

func Authenticate(token string) bool {
	fmt.Println("authenticating")
	return token != ""
}
`)

	// pkg/auth/auth_test.go
	impactWriteFile(t, filepath.Join(dir, "pkg", "auth", "auth_test.go"), `package auth

import "testing"

func TestAuthenticate(t *testing.T) {
	if !Authenticate("valid") {
		t.Fatal("expected true")
	}
}
`)

	// pkg/handler/handler.go - imports auth
	impactMkdirAll(t, filepath.Join(dir, "pkg", "handler"))
	impactWriteFile(t, filepath.Join(dir, "pkg", "handler", "handler.go"), `package handler

import "example.com/testproj/pkg/auth"

func Handle(token string) bool {
	return auth.Authenticate(token)
}
`)

	// pkg/handler/handler_test.go
	impactWriteFile(t, filepath.Join(dir, "pkg", "handler", "handler_test.go"), `package handler

import "testing"

func TestHandle(t *testing.T) {
	if !Handle("token") {
		t.Fatal("expected true")
	}
}
`)

	// pkg/server/server.go - imports handler
	impactMkdirAll(t, filepath.Join(dir, "pkg", "server"))
	impactWriteFile(t, filepath.Join(dir, "pkg", "server", "server.go"), `package server

import "example.com/testproj/pkg/handler"

func Start(token string) bool {
	return handler.Handle(token)
}
`)

	// pkg/api/api.go - imports auth
	impactMkdirAll(t, filepath.Join(dir, "pkg", "api"))
	impactWriteFile(t, filepath.Join(dir, "pkg", "api", "api.go"), `package api

import "example.com/testproj/pkg/auth"

func Validate(token string) bool {
	return auth.Authenticate(token)
}
`)

	// pkg/api/api_test.go
	impactWriteFile(t, filepath.Join(dir, "pkg", "api", "api_test.go"), `package api

import "testing"

func TestValidate(t *testing.T) {
	if !Validate("x") {
		t.Fatal("expected true")
	}
}
`)

	// cmd/app/main.go - imports server and api
	impactMkdirAll(t, filepath.Join(dir, "cmd", "app"))
	impactWriteFile(t, filepath.Join(dir, "cmd", "app", "main.go"), `package main

import (
	"example.com/testproj/pkg/server"
	"example.com/testproj/pkg/api"
	"fmt"
)

func main() {
	fmt.Println(server.Start("tok"))
	fmt.Println(api.Validate("tok"))
}
`)

	return dir
}

func impactMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("impactMkdirAll %s: %v", path, err)
	}
}

func impactWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("impactWriteFile %s: %v", path, err)
	}
}

func TestNewImpactAnalyzer(t *testing.T) {
	ia := NewImpactAnalyzer("/some/path")
	if ia == nil {
		t.Fatal("expected non-nil analyzer")
	}
	if ia.ProjectDir != "/some/path" {
		t.Fatalf("expected /some/path, got %s", ia.ProjectDir)
	}
	if ia.ImportGraph == nil {
		t.Fatal("expected non-nil ImportGraph")
	}
	if ia.TestMapping == nil {
		t.Fatal("expected non-nil TestMapping")
	}
}

func TestBuildImportGraph(t *testing.T) {
	dir := setupImpactTestProject(t)
	graph := BuildImportGraph(dir)

	// pkg/auth is imported by handler, api, and (indirectly) nothing else at this level.
	authPkg := "example.com/testproj/pkg/auth"
	dependents := graph[authPkg]

	if len(dependents) < 2 {
		t.Fatalf("expected at least 2 dependents of auth, got %d: %v", len(dependents), dependents)
	}

	// Check that handler and api are in the dependents.
	found := map[string]bool{}
	for _, d := range dependents {
		found[d] = true
	}
	if !found["example.com/testproj/pkg/handler"] {
		t.Error("expected pkg/handler to depend on pkg/auth")
	}
	if !found["example.com/testproj/pkg/api"] {
		t.Error("expected pkg/api to depend on pkg/auth")
	}
}

func TestFindDirectDependents(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)
	ia.ImportGraph = BuildImportGraph(dir)

	deps := ia.FindDirectDependents("example.com/testproj/pkg/auth")
	if len(deps) < 2 {
		t.Fatalf("expected at least 2 direct dependents, got %d", len(deps))
	}

	// handler and api should directly depend on auth.
	depSet := map[string]bool{}
	for _, d := range deps {
		depSet[d] = true
	}
	if !depSet["example.com/testproj/pkg/handler"] {
		t.Error("expected handler as direct dependent")
	}
	if !depSet["example.com/testproj/pkg/api"] {
		t.Error("expected api as direct dependent")
	}
}

func TestFindTransitiveDependents(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)
	ia.ImportGraph = BuildImportGraph(dir)

	deps := ia.FindTransitiveDependents("example.com/testproj/pkg/auth", 5)

	// auth -> handler -> server -> cmd/app (transitive chain)
	// auth -> api -> cmd/app
	depSet := map[string]bool{}
	for _, d := range deps {
		depSet[d] = true
	}

	if !depSet["example.com/testproj/pkg/handler"] {
		t.Error("expected handler in transitive dependents")
	}
	if !depSet["example.com/testproj/pkg/server"] {
		t.Error("expected server in transitive dependents")
	}
	if !depSet["example.com/testproj/cmd/app"] {
		t.Error("expected cmd/app in transitive dependents")
	}
}

func TestFindTransitiveDependentsDepthLimit(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)
	ia.ImportGraph = BuildImportGraph(dir)

	// With depth 1, only direct dependents should appear.
	deps := ia.FindTransitiveDependents("example.com/testproj/pkg/auth", 1)

	depSet := map[string]bool{}
	for _, d := range deps {
		depSet[d] = true
	}

	// handler and api are at depth 1.
	if !depSet["example.com/testproj/pkg/handler"] {
		t.Error("expected handler at depth 1")
	}
	// server is at depth 2 (auth -> handler -> server), should NOT be present.
	if depSet["example.com/testproj/pkg/server"] {
		t.Error("server should not appear at depth 1")
	}
}

func TestAnalyze(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)

	analysis, err := ia.Analyze([]string{"pkg/auth/auth.go"})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(analysis.ChangedFiles) != 1 {
		t.Fatalf("expected 1 changed file, got %d", len(analysis.ChangedFiles))
	}

	if len(analysis.DirectlyAffected) < 2 {
		t.Errorf("expected at least 2 directly affected, got %d: %v",
			len(analysis.DirectlyAffected), analysis.DirectlyAffected)
	}

	if analysis.RiskScore < 0 || analysis.RiskScore > 1 {
		t.Errorf("risk score out of range: %f", analysis.RiskScore)
	}

	if analysis.TestCoverage < 0 || analysis.TestCoverage > 1 {
		t.Errorf("test coverage out of range: %f", analysis.TestCoverage)
	}

	if len(analysis.Suggestions) == 0 {
		t.Error("expected at least one suggestion")
	}
}

func TestScoreRisk(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)
	ia.ImportGraph = BuildImportGraph(dir)
	ia.buildTestMapping()

	// Low risk: no affected packages.
	lowRisk := &ImpactAnalysis{
		ChangedFiles:         []string{"pkg/auth/auth.go"},
		DirectlyAffected:     nil,
		TransitivelyAffected: nil,
	}
	score := ia.ScoreRisk(lowRisk)
	if score > 0.5 {
		t.Errorf("expected low risk score, got %f", score)
	}

	// Higher risk: many affected packages including cmd.
	highRisk := &ImpactAnalysis{
		ChangedFiles: []string{"a.go", "b.go", "c.go", "d.go", "e.go"},
		DirectlyAffected: []string{
			"example.com/testproj/pkg/handler",
			"example.com/testproj/pkg/api",
			"example.com/testproj/pkg/server",
		},
		TransitivelyAffected: []string{
			"example.com/testproj/cmd/app",
			"example.com/testproj/pkg/extra1",
			"example.com/testproj/pkg/extra2",
			"example.com/testproj/pkg/extra3",
			"example.com/testproj/pkg/extra4",
			"example.com/testproj/pkg/extra5",
		},
	}
	score = ia.ScoreRisk(highRisk)
	if score < 0.5 {
		t.Errorf("expected high risk score, got %f", score)
	}
}

func TestFindTestCoverage(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)
	ia.ImportGraph = BuildImportGraph(dir)
	ia.buildTestMapping()

	// auth, handler, and api have tests; server and cmd/app do not.
	packages := []string{
		"example.com/testproj/pkg/auth",
		"example.com/testproj/pkg/handler",
		"example.com/testproj/pkg/server",
	}
	coverage := ia.FindTestCoverage(packages)
	// 2/3 have tests (auth, handler).
	expected := 2.0 / 3.0
	if coverage < expected-0.01 || coverage > expected+0.01 {
		t.Errorf("expected coverage ~%.2f, got %.2f", expected, coverage)
	}
}

func TestFindTestCoverageEmpty(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)

	// Empty packages list should return 1.0 (full coverage by default).
	coverage := ia.FindTestCoverage(nil)
	if coverage != 1.0 {
		t.Errorf("expected 1.0 for empty packages, got %f", coverage)
	}
}

func TestGenerateSuggestions(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)
	ia.ImportGraph = BuildImportGraph(dir)
	ia.buildTestMapping()

	analysis := &ImpactAnalysis{
		ChangedFiles: []string{"pkg/auth/auth.go"},
		DirectlyAffected: []string{
			"example.com/testproj/pkg/handler",
			"example.com/testproj/pkg/api",
		},
		TransitivelyAffected: []string{
			"example.com/testproj/cmd/app",
			"example.com/testproj/pkg/server",
		},
	}

	suggestions := ia.GenerateSuggestions(analysis)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions")
	}

	// Should have a "Run: go test" suggestion.
	hasRunSuggestion := false
	for _, s := range suggestions {
		if strings.HasPrefix(s, "Run: go test") {
			hasRunSuggestion = true
		}
	}
	if !hasRunSuggestion {
		t.Error("expected 'Run: go test' suggestion")
	}

	// Should mention cmd for integration impact.
	hasCmdReview := false
	for _, s := range suggestions {
		if strings.Contains(s, "cmd/app") && strings.Contains(s, "integration") {
			hasCmdReview = true
		}
	}
	if !hasCmdReview {
		t.Errorf("expected suggestion about cmd/app integration, got: %v", suggestions)
	}
}

func TestFormatImpact(t *testing.T) {
	analysis := &ImpactAnalysis{
		ChangedFiles:         []string{"pkg/auth/token.go", "pkg/auth/middleware.go"},
		DirectlyAffected:     []string{"pkg/handler/", "pkg/server/", "pkg/api/"},
		TransitivelyAffected: []string{"cmd/hawk/", "pkg/daemon/"},
		RiskScore:            0.78,
		TestCoverage:         0.67,
		Suggestions: []string{
			"Run: go test ./pkg/handler/... ./pkg/server/... ./pkg/api/...",
			"Review cmd/hawk/ for integration impact",
			"Consider adding tests for pkg/daemon/",
		},
	}

	output := FormatImpact(analysis)

	if !strings.Contains(output, "Change Impact Analysis:") {
		t.Error("expected header")
	}
	if !strings.Contains(output, "═══") {
		t.Error("expected separator")
	}
	if !strings.Contains(output, "HIGH") {
		t.Error("expected HIGH risk label")
	}
	if !strings.Contains(output, "0.78") {
		t.Error("expected risk score")
	}
	if !strings.Contains(output, "Direct dependents (3)") {
		t.Error("expected direct dependents count")
	}
	if !strings.Contains(output, "Transitive dependents (2)") {
		t.Error("expected transitive dependents count")
	}
	if !strings.Contains(output, "67%") {
		t.Error("expected test coverage percentage")
	}
	if !strings.Contains(output, "Suggestions:") {
		t.Error("expected suggestions section")
	}
}

func TestFormatImpactRiskLevels(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{0.1, "LOW"},
		{0.29, "LOW"},
		{0.3, "MEDIUM"},
		{0.59, "MEDIUM"},
		{0.6, "HIGH"},
		{0.99, "HIGH"},
	}

	for _, tc := range tests {
		analysis := &ImpactAnalysis{
			ChangedFiles: []string{"test.go"},
			RiskScore:    tc.score,
		}
		output := FormatImpact(analysis)
		if !strings.Contains(output, tc.expected) {
			t.Errorf("score %.2f: expected %s in output, got:\n%s", tc.score, tc.expected, output)
		}
	}
}

func TestQuickImpact(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)

	result := ia.QuickImpact(filepath.Join(dir, "pkg", "auth", "auth.go"))
	if !strings.Contains(result, "pkg/auth") {
		t.Errorf("expected pkg/auth in quick impact, got: %s", result)
	}
	if !strings.Contains(result, "direct") {
		t.Errorf("expected 'direct' in quick impact, got: %s", result)
	}
	if !strings.Contains(result, "transitive") {
		t.Errorf("expected 'transitive' in quick impact, got: %s", result)
	}
}

func TestQuickImpactRelativePath(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)

	result := ia.QuickImpact("pkg/auth/auth.go")
	if !strings.Contains(result, "pkg/auth") {
		t.Errorf("expected pkg/auth in quick impact, got: %s", result)
	}
}

func TestImpactDetectModulePath(t *testing.T) {
	dir := setupImpactTestProject(t)
	mod := detectModulePath(dir)
	if mod != "example.com/testproj" {
		t.Errorf("expected example.com/testproj, got %s", mod)
	}
}

func TestImpactDetectModulePathMissing(t *testing.T) {
	dir := t.TempDir()
	mod := detectModulePath(dir)
	if mod != "" {
		t.Errorf("expected empty string, got %s", mod)
	}
}

func TestFileToPackage(t *testing.T) {
	dir := setupImpactTestProject(t)

	tests := []struct {
		file     string
		expected string
	}{
		{"pkg/auth/auth.go", "example.com/testproj/pkg/auth"},
		{"pkg/handler/handler.go", "example.com/testproj/pkg/handler"},
		{"cmd/app/main.go", "example.com/testproj/cmd/app"},
	}

	for _, tc := range tests {
		got := impactFileToPackage(tc.file, dir)
		if got != tc.expected {
			t.Errorf("impactFileToPackage(%s) = %s, want %s", tc.file, got, tc.expected)
		}
	}
}

func TestImpactAppendUnique(t *testing.T) {
	s := []string{"a", "b", "c"}
	s = appendUnique(s, "b")
	if len(s) != 3 {
		t.Errorf("expected 3 elements after adding duplicate, got %d", len(s))
	}
	s = appendUnique(s, "d")
	if len(s) != 4 {
		t.Errorf("expected 4 elements after adding new, got %d", len(s))
	}
}

func TestHasTestFiles(t *testing.T) {
	dir := setupImpactTestProject(t)

	// pkg/auth has test files.
	if !hasTestFiles(filepath.Join(dir, "pkg", "auth")) {
		t.Error("expected pkg/auth to have test files")
	}

	// pkg/server does not have test files.
	if hasTestFiles(filepath.Join(dir, "pkg", "server")) {
		t.Error("expected pkg/server to NOT have test files")
	}

	// Non-existent directory.
	if hasTestFiles(filepath.Join(dir, "nonexistent")) {
		t.Error("expected nonexistent dir to return false")
	}
}

func TestAnalyzeConcurrency(t *testing.T) {
	dir := setupImpactTestProject(t)
	ia := NewImpactAnalyzer(dir)

	// Run multiple analyses concurrently to test thread safety.
	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, err := ia.Analyze([]string{"pkg/auth/auth.go"})
			if err != nil {
				t.Errorf("concurrent Analyze failed: %v", err)
			}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestBuildImportGraphSkipsVendor(t *testing.T) {
	dir := setupImpactTestProject(t)

	// Create a vendor directory with a file that imports auth.
	vendorDir := filepath.Join(dir, "vendor", "somepkg")
	impactMkdirAll(t, vendorDir)
	impactWriteFile(t, filepath.Join(vendorDir, "v.go"), `package somepkg

import "example.com/testproj/pkg/auth"

var _ = auth.Authenticate
`)

	graph := BuildImportGraph(dir)
	authDeps := graph["example.com/testproj/pkg/auth"]
	for _, d := range authDeps {
		if strings.Contains(d, "vendor") {
			t.Errorf("vendor package should be excluded from graph, found: %s", d)
		}
	}
}
