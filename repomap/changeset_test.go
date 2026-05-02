package repomap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── Helper: create a git repo with changes ──

// createGitRepoWithChanges sets up a temp git repo with a Go module,
// commits initial files, then modifies a file to create a diff.
func createGitRepoWithChanges(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Init git repo
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@test.com")
	runGit(t, root, "config", "user.name", "Test")

	// Create module structure
	writeFile(t, root, "go.mod", "module mymod\n\ngo 1.21\n")
	writeFile(t, root, "main.go", `package main

import (
	"fmt"
	"mymod/pkg/auth"
)

func main() {
	fmt.Println(auth.Check())
}
`)
	writeFile(t, root, "pkg/auth/auth.go", `package auth

import "mymod/pkg/models"

func Check() bool {
	_ = models.User{}
	return true
}
`)
	writeFile(t, root, "pkg/models/user.go", `package models

type User struct {
	Name string
}
`)
	writeFile(t, root, "pkg/api/routes.go", `package api

import "mymod/pkg/auth"

func SetupRoutes() {
	_ = auth.Check()
}
`)

	// Commit everything
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "initial")

	// Modify auth.go to create a working tree change
	writeFile(t, root, "pkg/auth/auth.go", `package auth

import "mymod/pkg/models"

func Check() bool {
	_ = models.User{}
	return true
}

func Verify() bool {
	return false
}
`)

	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// ── FromGitDiff tests ──

func TestFromGitDiff_DetectsChangedFiles(t *testing.T) {
	root := createGitRepoWithChanges(t)

	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiff(root, graph)
	if err != nil {
		t.Fatalf("FromGitDiff failed: %v", err)
	}

	// auth.go was modified
	if len(ctx.ChangedFiles) != 1 {
		t.Fatalf("expected 1 changed file, got %d: %v", len(ctx.ChangedFiles), ctx.ChangedFiles)
	}
	if ctx.ChangedFiles[0] != filepath.Join("pkg", "auth", "auth.go") {
		t.Errorf("expected pkg/auth/auth.go changed, got %v", ctx.ChangedFiles)
	}
}

func TestFromGitDiff_FindsImpactedFiles(t *testing.T) {
	root := createGitRepoWithChanges(t)

	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiff(root, graph)
	if err != nil {
		t.Fatal(err)
	}

	// Files that import auth should be impacted
	assertContains(t, ctx.ImpactedFiles, "main.go", "main.go imports auth")
	assertContains(t, ctx.ImpactedFiles, filepath.Join("pkg", "api", "routes.go"), "routes.go imports auth")
}

func TestFromGitDiff_FindsDependencyFiles(t *testing.T) {
	root := createGitRepoWithChanges(t)

	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiff(root, graph)
	if err != nil {
		t.Fatal(err)
	}

	// auth.go imports models, so models should be a dependency
	assertContains(t, ctx.DependencyFiles, filepath.Join("pkg", "models", "user.go"),
		"models/user.go is a dependency of auth.go")
}

func TestFromGitDiff_TotalFilesCount(t *testing.T) {
	root := createGitRepoWithChanges(t)

	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiff(root, graph)
	if err != nil {
		t.Fatal(err)
	}

	expectedTotal := len(ctx.ChangedFiles) + len(ctx.ImpactedFiles) + len(ctx.DependencyFiles)
	if ctx.TotalFiles != expectedTotal {
		t.Errorf("TotalFiles %d != sum of parts %d", ctx.TotalFiles, expectedTotal)
	}
}

// ── FromGitDiffRange tests ──

func TestFromGitDiffRange_BranchComparison(t *testing.T) {
	root := createGitRepoWithChanges(t)

	// Commit the change on a branch
	runGit(t, root, "checkout", "-b", "feature")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "add verify")

	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiffRange(root, "main..feature", graph)
	if err != nil {
		t.Fatalf("FromGitDiffRange failed: %v", err)
	}

	if len(ctx.ChangedFiles) != 1 {
		t.Fatalf("expected 1 changed file in range, got %d: %v", len(ctx.ChangedFiles), ctx.ChangedFiles)
	}
	if ctx.ChangedFiles[0] != filepath.Join("pkg", "auth", "auth.go") {
		t.Errorf("expected pkg/auth/auth.go, got %v", ctx.ChangedFiles)
	}
}

// ── FromGitDiff with nil graph ──

func TestFromGitDiff_NilGraph(t *testing.T) {
	root := createGitRepoWithChanges(t)

	ctx, err := FromGitDiff(root, nil)
	if err != nil {
		t.Fatalf("FromGitDiff with nil graph should work: %v", err)
	}

	if len(ctx.ChangedFiles) == 0 {
		t.Error("expected changed files even with nil graph")
	}
	if len(ctx.ImpactedFiles) != 0 {
		t.Errorf("expected no impacted files with nil graph, got %v", ctx.ImpactedFiles)
	}
	if len(ctx.DependencyFiles) != 0 {
		t.Errorf("expected no dependency files with nil graph, got %v", ctx.DependencyFiles)
	}
}

// ── FormatContext tests ──

func TestFormatContext_Basic(t *testing.T) {
	ctx := &ChangeSetContext{
		ChangedFiles:    []string{"pkg/auth/handler.go", "pkg/auth/middleware.go"},
		ImpactedFiles:   []string{"cmd/server/main.go", "pkg/api/routes.go"},
		DependencyFiles: []string{"pkg/models/user.go"},
		TotalFiles:      5,
	}

	output := ctx.FormatContext(2000)

	// Verify all sections appear
	if !strings.Contains(output, "## Changed Files (direct edits)") {
		t.Error("missing Changed Files section")
	}
	if !strings.Contains(output, "## Impacted Files (depend on changes)") {
		t.Error("missing Impacted Files section")
	}
	if !strings.Contains(output, "## Dependencies (needed for context)") {
		t.Error("missing Dependencies section")
	}

	// Verify specific files appear
	if !strings.Contains(output, "pkg/auth/handler.go") {
		t.Error("missing handler.go in output")
	}
	if !strings.Contains(output, "pkg/models/user.go") {
		t.Error("missing user.go in output")
	}
}

func TestFormatContext_Nil(t *testing.T) {
	var ctx *ChangeSetContext
	output := ctx.FormatContext(2000)
	if output != "" {
		t.Errorf("expected empty output for nil context, got %q", output)
	}
}

func TestFormatContext_EmptyChanges(t *testing.T) {
	ctx := &ChangeSetContext{}
	output := ctx.FormatContext(2000)
	if output != "" {
		t.Errorf("expected empty output for empty context, got %q", output)
	}
}

func TestFormatContext_TokenLimit(t *testing.T) {
	// Create a context with many files to trigger truncation
	var changed []string
	for i := 0; i < 100; i++ {
		changed = append(changed, "pkg/very/long/path/file"+strings.Repeat("x", 50)+".go")
	}

	ctx := &ChangeSetContext{
		ChangedFiles: changed,
		TotalFiles:   100,
	}

	output := ctx.FormatContext(100) // very small token budget
	if !strings.Contains(output, "more changed files") {
		t.Error("expected truncation message for small token budget")
	}
}

func TestFormatContext_AllSections(t *testing.T) {
	ctx := &ChangeSetContext{
		ChangedFiles:    []string{"a.go"},
		ImpactedFiles:   []string{"b.go"},
		DependencyFiles: []string{"c.go"},
		TotalFiles:      3,
	}

	output := ctx.FormatContext(2000)

	sections := []string{"## Changed Files", "## Impacted Files", "## Dependencies"}
	for _, sec := range sections {
		if !strings.Contains(output, sec) {
			t.Errorf("missing section %q in output:\n%s", sec, output)
		}
	}
}

// ── buildContext unit tests ──

func TestBuildContext_NilGraph(t *testing.T) {
	ctx, err := buildContext("/tmp", []string{"a.go", "b.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.ChangedFiles) != 2 {
		t.Errorf("expected 2 changed files, got %d", len(ctx.ChangedFiles))
	}
	if ctx.TotalFiles != 2 {
		t.Errorf("expected TotalFiles 2, got %d", ctx.TotalFiles)
	}
}

func TestBuildContext_NoDuplicates(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// Both main.go and routes.go depend on auth. If we change auth,
	// main and routes should each appear exactly once in impacted.
	ctx, err := buildContext(root, []string{"pkg/auth/auth.go"}, g)
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]int)
	for _, f := range ctx.ImpactedFiles {
		seen[f]++
	}
	for _, f := range ctx.DependencyFiles {
		seen[f]++
	}
	for _, f := range ctx.ChangedFiles {
		seen[f]++
	}

	for f, count := range seen {
		if count > 1 {
			t.Errorf("file %q appears %d times across context sets", f, count)
		}
	}
}

// ── Integration: end-to-end git diff + import graph ──

func TestEndToEnd_GitDiffWithImportGraph(t *testing.T) {
	root := createGitRepoWithChanges(t)

	// Build graph and context
	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiff(root, graph)
	if err != nil {
		t.Fatal(err)
	}

	// Format and verify output
	output := ctx.FormatContext(2000)
	if output == "" {
		t.Fatal("expected non-empty formatted output")
	}

	// The output should mention auth.go as changed
	if !strings.Contains(output, "auth.go") {
		t.Error("formatted output should mention auth.go")
	}

	// Total files should be reasonable (1 changed + 2 impacted + 1 dependency = 4)
	if ctx.TotalFiles < 2 {
		t.Errorf("expected at least 2 total files, got %d", ctx.TotalFiles)
	}
}

// ── gitDiffFiles edge case ──

func TestGitDiffFiles_CleanRepo(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@test.com")
	runGit(t, root, "config", "user.name", "Test")

	writeFile(t, root, "main.go", "package main\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")

	// No changes -- should return empty
	files, err := gitDiffFiles(root, "")
	if err != nil {
		t.Fatalf("gitDiffFiles on clean repo: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no changed files in clean repo, got %v", files)
	}
}

func TestGitDiffFiles_StagedChanges(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@test.com")
	runGit(t, root, "config", "user.name", "Test")

	writeFile(t, root, "main.go", "package main\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")

	// Modify and stage
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	runGit(t, root, "add", "main.go")

	files, err := gitDiffFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "main.go" {
		t.Errorf("expected [main.go], got %v", files)
	}
}

// ── estimateLineTokens tests ──

func TestEstimateLineTokens(t *testing.T) {
	if estimateLineTokens("") != 1 {
		t.Error("empty string should estimate to 1 token minimum")
	}
	if estimateLineTokens("a short line") < 1 {
		t.Error("should always return at least 1")
	}

	long := strings.Repeat("word ", 100)
	tokens := estimateLineTokens(long)
	if tokens < 50 {
		t.Errorf("expected >= 50 tokens for long line, got %d", tokens)
	}
}

// ── sortedKeys tests ──

func TestSortedKeys(t *testing.T) {
	m := map[string]bool{"c": true, "a": true, "b": true}
	got := sortedKeys(m)
	expected := []string{"a", "b", "c"}
	if len(got) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("sortedKeys[%d]: expected %q, got %q", i, expected[i], got[i])
		}
	}
}

func TestSortedKeys_Empty(t *testing.T) {
	got := sortedKeys(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

// ── gitDiffFiles with initial commit (no HEAD) ──

func TestGitDiffFiles_InitialCommit(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@test.com")
	runGit(t, root, "config", "user.name", "Test")

	// Stage a file but don't commit (no HEAD exists)
	writeFile(t, root, "main.go", "package main\n")
	runGit(t, root, "add", "main.go")

	files, err := gitDiffFiles(root, "")
	if err != nil {
		t.Fatalf("gitDiffFiles on initial commit: %v", err)
	}

	// Should pick up the staged file via --cached fallback
	if len(files) != 1 || files[0] != "main.go" {
		t.Errorf("expected [main.go] from staged-only diff, got %v", files)
	}
}

// ── Verify no git binary needed for buildContext ──

func TestBuildContext_DoesNotRequireGit(t *testing.T) {
	root := createGoModule(t)
	g, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	// buildContext itself does not call git -- it takes pre-computed files
	ctx, err := buildContext(root, []string{"pkg/auth/auth.go"}, g)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.TotalFiles == 0 {
		t.Error("expected non-zero total files")
	}
}

// ── Verify that ChangeSetContext is smaller than full file list ──

func TestChangeSetContext_SmallerThanFull(t *testing.T) {
	root := createGitRepoWithChanges(t)

	graph, err := BuildImportGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := FromGitDiff(root, graph)
	if err != nil {
		t.Fatal(err)
	}

	// Count total files in repo
	totalRepoFiles := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if isSupportedExt(filepath.Ext(path)) {
			totalRepoFiles++
		}
		return nil
	})

	// The context set should be a subset of all files
	if ctx.TotalFiles > totalRepoFiles {
		t.Errorf("context set (%d) should be <= total repo files (%d)",
			ctx.TotalFiles, totalRepoFiles)
	}
}
