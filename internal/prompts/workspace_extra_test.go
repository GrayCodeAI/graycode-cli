package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- readGitBranch tests ---

func TestReadGitBranch_NormalRepo(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	headContent := "ref: refs/heads/main\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "main" {
		t.Errorf("readGitBranch() = %q, want %q", branch, "main")
	}
}

func TestReadGitBranch_DetachedHEAD(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	headContent := "abc1234567890abcdef\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "abc12345 (detached)" {
		t.Errorf("readGitBranch() = %q, want %q", branch, "abc12345 (detached)")
	}
}

func TestReadGitBranch_ShortHash(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	headContent := "abc123\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "" {
		t.Errorf("readGitBranch() = %q, want empty for short hash", branch)
	}
}

func TestReadGitBranch_NonExistentGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	branch := readGitBranch(tmpDir)
	if branch != "" {
		t.Errorf("readGitBranch() = %q, want empty for non-existent .git", branch)
	}
}

func TestReadGitBranch_WorktreeFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create .git file (worktree) pointing to gitdir
	gitFileContent := "gitdir: /tmp/fake-git-dir\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".git"), []byte(gitFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "" {
		t.Errorf("readGitBranch() = %q, want empty for non-existent worktree gitdir", branch)
	}
}

func TestReadGitBranch_WorktreeFile_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a real git dir with HEAD
	gitDir := filepath.Join(tmpDir, "custom-git-dir")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	headContent := "ref: refs/heads/develop\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .git file pointing to the git dir
	gitFileContent := "gitdir: " + gitDir + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".git"), []byte(gitFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "develop" {
		t.Errorf("readGitBranch() = %q, want %q", branch, "develop")
	}
}

func TestReadGitBranch_WorktreeFile_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a real git dir with HEAD
	gitDir := filepath.Join(tmpDir, "custom-git-dir")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	headContent := "ref: refs/heads/feature\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .git file pointing to the git dir (relative path)
	gitFileContent := "gitdir: custom-git-dir\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".git"), []byte(gitFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "feature" {
		t.Errorf("readGitBranch() = %q, want %q", branch, "feature")
	}
}

func TestReadGitBranch_WorktreeFile_NonRefContent(t *testing.T) {
	tmpDir := t.TempDir()
	// Create .git file with non-ref content
	gitFileContent := "some other content\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".git"), []byte(gitFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	branch := readGitBranch(tmpDir)
	if branch != "" {
		t.Errorf("readGitBranch() = %q, want empty for non-ref content", branch)
	}
}

// --- Format tests ---

func TestWorkspaceContext_Format_Nil(t *testing.T) {
	var w *WorkspaceContext
	result := w.Format()
	if result != "" {
		t.Errorf("Format() on nil = %q, want empty", result)
	}
}

func TestWorkspaceContext_Format_Empty(t *testing.T) {
	w := &WorkspaceContext{}
	result := w.Format()
	if !strings.Contains(result, "## Project Context") {
		t.Errorf("Format() should contain '## Project Context', got %q", result)
	}
}

func TestWorkspaceContext_Format_WithBranch(t *testing.T) {
	w := &WorkspaceContext{
		GitBranch: "main",
		GitStatus: "clean",
	}
	result := w.Format()
	if !strings.Contains(result, "Branch: main (clean)") {
		t.Errorf("Format() should contain 'Branch: main (clean)', got %q", result)
	}
}

func TestWorkspaceContext_Format_WithBranchNoStatus(t *testing.T) {
	w := &WorkspaceContext{
		GitBranch: "main",
	}
	result := w.Format()
	if !strings.Contains(result, "Branch: main") {
		t.Errorf("Format() should contain 'Branch: main', got %q", result)
	}
}

func TestWorkspaceContext_Format_WithRecentCommits(t *testing.T) {
	w := &WorkspaceContext{
		RecentCommits: []string{"fix bug", "add feature"},
	}
	result := w.Format()
	if !strings.Contains(result, "Recent: fix bug / add feature") {
		t.Errorf("Format() should contain recent commits, got %q", result)
	}
}

func TestWorkspaceContext_Format_WithTopFiles(t *testing.T) {
	w := &WorkspaceContext{
		TopFiles: []string{"main.go", "utils.go"},
		Language: "Go",
	}
	result := w.Format()
	if !strings.Contains(result, "Structure: main.go utils.go (Go project)") {
		t.Errorf("Format() should contain structure, got %q", result)
	}
}

func TestWorkspaceContext_Format_WithTopFilesNoLanguage(t *testing.T) {
	w := &WorkspaceContext{
		TopFiles: []string{"main.go"},
	}
	result := w.Format()
	if !strings.Contains(result, "Structure: main.go") {
		t.Errorf("Format() should contain structure, got %q", result)
	}
}

func TestWorkspaceContext_Format_TopFilesTruncated(t *testing.T) {
	w := &WorkspaceContext{
		TopFiles: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
	}
	result := w.Format()
	if !strings.Contains(result, "Structure: a b c d e f g h i j") {
		t.Errorf("Format() should truncate to 10 files, got %q", result)
	}
}

func TestWorkspaceContext_Format_WithChangedFiles(t *testing.T) {
	w := &WorkspaceContext{
		ChangedFiles: []string{"file1.go", "file2.go"},
	}
	result := w.Format()
	if !strings.Contains(result, "Changed: file1.go, file2.go") {
		t.Errorf("Format() should contain changed files, got %q", result)
	}
}

func TestWorkspaceContext_Format_ChangedFilesTruncated(t *testing.T) {
	w := &WorkspaceContext{
		ChangedFiles: []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"},
	}
	result := w.Format()
	if !strings.Contains(result, "+2 more") {
		t.Errorf("Format() should show truncation, got %q", result)
	}
}

// --- loadTemplateSource tests ---

func TestLoadTemplateSource_NonExistent(t *testing.T) {
	_, err := loadTemplateSource("nonexistent-template-xyz.md")
	if err == nil {
		t.Error("expected error for non-existent template")
	}
}

func TestLoadTemplateSource_EmbeddedTemplate(t *testing.T) {
	// Test loading an embedded template
	source, err := loadTemplateSource("role.md")
	if err != nil {
		t.Fatalf("loadTemplateSource(role.md) error: %v", err)
	}
	if source == "" {
		t.Error("expected non-empty template source")
	}
}

// --- loadTemplateForRender tests ---

func TestLoadTemplateForRender_NonExistent(t *testing.T) {
	_, err := loadTemplateForRender("nonexistent-template-xyz.md")
	if err == nil {
		t.Error("expected error for non-existent template")
	}
}

func TestLoadTemplateForRender_EmbeddedTemplate(t *testing.T) {
	tmpl, err := loadTemplateForRender("role.md")
	if err != nil {
		t.Fatalf("loadTemplateForRender(role.md) error: %v", err)
	}
	if tmpl == nil {
		t.Error("expected non-nil template")
	}
}

func TestLoadTemplateForRender_Caching(t *testing.T) {
	// First call should cache
	tmpl1, err := loadTemplateForRender("subagent.md")
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	// Second call should return cached version
	tmpl2, err := loadTemplateForRender("subagent.md")
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if tmpl1 != tmpl2 {
		t.Error("expected same template instance from cache")
	}
}

// --- renderTemplate tests ---

func TestRenderTemplate(t *testing.T) {
	tmpl, err := loadTemplateForRender("role.md")
	if err != nil {
		t.Fatalf("loadTemplateForRender error: %v", err)
	}

	ctx := PromptContext{
		Date:    "Monday, 2024-01-01",
		WorkDir: "/test",
		OS:      "linux",
	}

	result, err := renderTemplate("role.md", tmpl, ctx)
	if err != nil {
		t.Fatalf("renderTemplate error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty rendered template")
	}
}

// --- BuildSubAgentPrompt tests ---

func TestBuildSubAgentPrompt_WithAllFields(t *testing.T) {
	ctx := PromptContext{
		Date:    "Monday, 2024-01-01",
		WorkDir: "/test",
		OS:      "linux",
		Shell:   "/bin/bash",
		Model:   "gpt-4",
		Provider: "openai",
		GitBranch: "main",
		GitStatus: "clean",
		RecentCommits: "fix bug",
		TopFiles: "main.go",
		MaxTurns: 10,
		Task: "test task",
	}

	result, err := BuildSubAgentPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildSubAgentPrompt error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

// --- ListTemplates tests ---

func TestListTemplates_Empty(t *testing.T) {
	templates := ListTemplates()
	if len(templates) == 0 {
		t.Fatal("expected non-empty template list")
	}
}

// --- GatherWorkspaceContext tests ---

func TestGatherWorkspaceContext_NonExistentDir(t *testing.T) {
	ctx := GatherWorkspaceContext("/nonexistent-directory-xyz")
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.GitBranch != "" {
		t.Errorf("GitBranch = %q, want empty", ctx.GitBranch)
	}
}

func TestGatherWorkspaceContext_WithGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a fake git repo structure
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	headContent := "ref: refs/heads/main\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create some files
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "utils.py"), []byte("# utils"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := GatherWorkspaceContext(tmpDir)
	if ctx.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", ctx.GitBranch, "main")
	}
	if ctx.Language != "Go" {
		t.Errorf("Language = %q, want %q", ctx.Language, "Go")
	}
	if len(ctx.TopFiles) == 0 {
		t.Error("expected non-empty TopFiles")
	}
}

// --- detectLanguage tests ---

func TestDetectLanguage_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	lang := detectLanguage(tmpDir)
	if lang != "" {
		t.Errorf("detectLanguage() = %q, want empty", lang)
	}
}

func TestDetectLanguage_NonExistentDir(t *testing.T) {
	lang := detectLanguage("/nonexistent-directory-xyz")
	if lang != "" {
		t.Errorf("detectLanguage() = %q, want empty", lang)
	}
}

func TestDetectLanguage_Python(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.py"), []byte("print('hello')"), 0644); err != nil {
		t.Fatal(err)
	}
	lang := detectLanguage(tmpDir)
	if lang != "Python" {
		t.Errorf("detectLanguage() = %q, want %q", lang, "Python")
	}
}

func TestDetectLanguage_Typescript(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "app.ts"), []byte("// typescript"), 0644); err != nil {
		t.Fatal(err)
	}
	lang := detectLanguage(tmpDir)
	if lang != "TypeScript" {
		t.Errorf("detectLanguage() = %q, want %q", lang, "TypeScript")
	}
}

func TestDetectLanguage_WithSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a subdirectory with Go files
	subDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	lang := detectLanguage(tmpDir)
	if lang != "Go" {
		t.Errorf("detectLanguage() = %q, want %q", lang, "Go")
	}
}

func TestDetectLanguage_SkipsNodeModules(t *testing.T) {
	tmpDir := t.TempDir()
	// Create node_modules with JS files (should be skipped)
	nodeModules := filepath.Join(tmpDir, "node_modules")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "lib.js"), []byte("// js"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a Python file in the root
	if err := os.WriteFile(filepath.Join(tmpDir, "main.py"), []byte("# python"), 0644); err != nil {
		t.Fatal(err)
	}
	lang := detectLanguage(tmpDir)
	if lang != "Python" {
		t.Errorf("detectLanguage() = %q, want %q (node_modules should be skipped)", lang, "Python")
	}
}

func TestDetectLanguage_UnknownExtension(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "file.xyz"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	lang := detectLanguage(tmpDir)
	if lang != "" {
		t.Errorf("detectLanguage() = %q, want empty for unknown extension", lang)
	}
}

// --- gitCmd tests ---

func TestGitCmd_NonExistentDir(t *testing.T) {
	_, err := gitCmd("/nonexistent-directory-xyz", "status")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestGitCmd_ValidDir(t *testing.T) {
	// Run git in the current directory (which is a git repo)
	_, err := gitCmd(".", "status", "--short")
	// This might succeed or fail depending on the environment, but shouldn't panic
	_ = err
}
