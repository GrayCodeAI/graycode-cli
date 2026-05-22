package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConventionalCommit_Basic(t *testing.T) {
	tests := []struct {
		name      string
		msg       string
		wantType  string
		wantScope string
		wantDesc  string
		wantBreak bool
	}{
		{
			name:      "simple feat",
			msg:       "feat: add new parser",
			wantType:  "feat",
			wantScope: "",
			wantDesc:  "add new parser",
			wantBreak: false,
		},
		{
			name:      "feat with scope",
			msg:       "feat(auth): add JWT refresh",
			wantType:  "feat",
			wantScope: "auth",
			wantDesc:  "add JWT refresh",
			wantBreak: false,
		},
		{
			name:      "fix with scope",
			msg:       "fix(session): resolve memory leak",
			wantType:  "fix",
			wantScope: "session",
			wantDesc:  "resolve memory leak",
			wantBreak: false,
		},
		{
			name:      "breaking with bang",
			msg:       "feat(api)!: remove legacy endpoint",
			wantType:  "feat",
			wantScope: "api",
			wantDesc:  "remove legacy endpoint",
			wantBreak: true,
		},
		{
			name:      "refactor no scope",
			msg:       "refactor: simplify error handling",
			wantType:  "refactor",
			wantScope: "",
			wantDesc:  "simplify error handling",
			wantBreak: false,
		},
		{
			name:      "perf with scope",
			msg:       "perf(cache): reduce allocations",
			wantType:  "perf",
			wantScope: "cache",
			wantDesc:  "reduce allocations",
			wantBreak: false,
		},
		{
			name:      "docs",
			msg:       "docs: update API reference",
			wantType:  "docs",
			wantScope: "",
			wantDesc:  "update API reference",
			wantBreak: false,
		},
		{
			name:      "test with scope",
			msg:       "test(utils): add edge case tests",
			wantType:  "test",
			wantScope: "utils",
			wantDesc:  "add edge case tests",
			wantBreak: false,
		},
		{
			name:      "chore",
			msg:       "chore: update dependencies",
			wantType:  "chore",
			wantScope: "",
			wantDesc:  "update dependencies",
			wantBreak: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseConventionalCommit(tt.msg)
			if entry == nil {
				t.Fatal("expected non-nil entry")
			}
			if entry.Type != tt.wantType {
				t.Errorf("type = %q, want %q", entry.Type, tt.wantType)
			}
			if entry.Scope != tt.wantScope {
				t.Errorf("scope = %q, want %q", entry.Scope, tt.wantScope)
			}
			if entry.Description != tt.wantDesc {
				t.Errorf("description = %q, want %q", entry.Description, tt.wantDesc)
			}
			if entry.Breaking != tt.wantBreak {
				t.Errorf("breaking = %v, want %v", entry.Breaking, tt.wantBreak)
			}
		})
	}
}

func TestParseConventionalCommit_BreakingInBody(t *testing.T) {
	msg := "feat(api): change auth flow\n\nBREAKING CHANGE: token format has changed"
	entry := ParseConventionalCommit(msg)
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if !entry.Breaking {
		t.Error("expected breaking=true from body BREAKING CHANGE")
	}
	if entry.Type != "feat" {
		t.Errorf("type = %q, want feat", entry.Type)
	}
}

func TestParseConventionalCommit_NonConventional(t *testing.T) {
	msg := "Update the readme file"
	entry := ParseConventionalCommit(msg)
	if entry != nil {
		t.Error("expected nil for non-conventional commit")
	}
}

func TestParseConventionalCommit_InvalidFormats(t *testing.T) {
	msgs := []string{
		"",
		"just a message",
		"no-colon here",
		": missing type",
	}
	for _, msg := range msgs {
		entry := ParseConventionalCommit(msg)
		if entry != nil {
			t.Errorf("expected nil for message %q, got %+v", msg, entry)
		}
	}
}

func TestBumpVersion_Major(t *testing.T) {
	changes := []ChangeEntry{
		{Type: "feat", Breaking: true, Description: "remove old API"},
		{Type: "fix", Description: "fix bug"},
	}
	result := BumpVersion("1.2.3", changes)
	if result != "2.0.0" {
		t.Errorf("got %s, want 2.0.0", result)
	}
}

func TestBumpVersion_MajorPreV1(t *testing.T) {
	changes := []ChangeEntry{
		{Type: "feat", Breaking: true, Description: "breaking change"},
	}
	result := BumpVersion("0.5.2", changes)
	if result != "0.6.0" {
		t.Errorf("got %s, want 0.6.0 (pre-1.0 breaking bumps minor)", result)
	}
}

func TestBumpVersion_Minor(t *testing.T) {
	changes := []ChangeEntry{
		{Type: "feat", Description: "new feature"},
		{Type: "fix", Description: "fix bug"},
	}
	result := BumpVersion("1.2.3", changes)
	if result != "1.3.0" {
		t.Errorf("got %s, want 1.3.0", result)
	}
}

func TestBumpVersion_Patch(t *testing.T) {
	changes := []ChangeEntry{
		{Type: "fix", Description: "fix bug"},
		{Type: "chore", Description: "update deps"},
	}
	result := BumpVersion("1.2.3", changes)
	if result != "1.2.4" {
		t.Errorf("got %s, want 1.2.4", result)
	}
}

func TestBumpVersion_InvalidCurrent(t *testing.T) {
	changes := []ChangeEntry{{Type: "feat", Description: "something"}}
	result := BumpVersion("invalid", changes)
	if result != "0.1.0" {
		t.Errorf("got %s, want 0.1.0 for invalid current version", result)
	}
}

func TestBumpVersion_FromZero(t *testing.T) {
	changes := []ChangeEntry{{Type: "feat", Description: "initial feature"}}
	result := BumpVersion("0.0.0", changes)
	if result != "0.1.0" {
		t.Errorf("got %s, want 0.1.0", result)
	}
}

func TestGenerateChangelog(t *testing.T) {
	release := &Release{
		Version: "1.3.0",
		Date:    time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		Changes: []ChangeEntry{
			{Type: "feat", Scope: "auth", Description: "Add JWT token refresh", CommitHash: "abc1234567"},
			{Type: "feat", Scope: "api", Description: "Add rate limiting middleware", CommitHash: "def5678901"},
			{Type: "fix", Scope: "session", Description: "Fix memory leak in long sessions", CommitHash: "ghi2345678"},
			{Type: "fix", Scope: "config", Description: "Handle missing config file gracefully", CommitHash: "jkl3456789"},
		},
		BreakingChanges: []ChangeEntry{
			{Type: "feat", Scope: "api", Description: "Remove deprecated /v1/legacy endpoint", Breaking: true},
		},
		Stats: ReleaseStats{
			Commits:      15,
			FilesChanged: 42,
			Additions:    1234,
			Deletions:    567,
			Contributors: 3,
		},
	}

	result := GenerateChangelog(release)

	// Check header
	if !strings.Contains(result, "## v1.3.0 (2024-03-15)") {
		t.Error("missing version header")
	}

	// Check features section
	if !strings.Contains(result, "### Features") {
		t.Error("missing Features section")
	}
	if !strings.Contains(result, "**auth**: Add JWT token refresh") {
		t.Error("missing auth feature")
	}
	if !strings.Contains(result, "**api**: Add rate limiting middleware") {
		t.Error("missing api feature")
	}

	// Check bug fixes section
	if !strings.Contains(result, "### Bug Fixes") {
		t.Error("missing Bug Fixes section")
	}
	if !strings.Contains(result, "**session**: Fix memory leak") {
		t.Error("missing session fix")
	}

	// Check breaking changes
	if !strings.Contains(result, "### Breaking Changes") {
		t.Error("missing Breaking Changes section")
	}
	if !strings.Contains(result, "Remove deprecated /v1/legacy endpoint") {
		t.Error("missing breaking change description")
	}

	// Check stats
	if !strings.Contains(result, "### Stats") {
		t.Error("missing Stats section")
	}
	if !strings.Contains(result, "15 commits") {
		t.Error("missing commit count")
	}
	if !strings.Contains(result, "42 files changed") {
		t.Error("missing files changed")
	}
	if !strings.Contains(result, "3 contributors") {
		t.Error("missing contributor count")
	}
}

func TestGenerateChangelog_NoBreaking(t *testing.T) {
	release := &Release{
		Version: "1.0.1",
		Date:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Changes: []ChangeEntry{
			{Type: "fix", Description: "fix a bug"},
		},
		BreakingChanges: nil,
		Stats:           ReleaseStats{Commits: 1, FilesChanged: 1, Additions: 5, Deletions: 2, Contributors: 1},
	}

	result := GenerateChangelog(release)
	if strings.Contains(result, "### Breaking Changes") {
		t.Error("should not have Breaking Changes section")
	}
	if !strings.Contains(result, "### Bug Fixes") {
		t.Error("missing Bug Fixes section")
	}
}

func TestGenerateChangelog_CommitHashTruncation(t *testing.T) {
	release := &Release{
		Version: "2.0.0",
		Date:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Changes: []ChangeEntry{
			{Type: "feat", Description: "something", CommitHash: "abcdef1234567890"},
		},
		Stats: ReleaseStats{Commits: 1, FilesChanged: 1, Additions: 10, Deletions: 0, Contributors: 1},
	}

	result := GenerateChangelog(release)
	if !strings.Contains(result, "(abcdef1)") {
		t.Error("commit hash should be truncated to 7 chars")
	}
}

func TestFormatReleaseNotes(t *testing.T) {
	release := &Release{
		Version: "2.0.0",
		Date:    time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		Changes: []ChangeEntry{
			{Type: "feat", Scope: "core", Description: "new engine", CommitHash: "abc1234567", Author: "alice"},
			{Type: "fix", Description: "fix crash", CommitHash: "def5678901", Author: "bob"},
		},
		BreakingChanges: []ChangeEntry{
			{Type: "feat", Scope: "core", Description: "new engine", Breaking: true},
		},
		Contributors: []string{"alice", "bob"},
		Stats: ReleaseStats{
			Commits:      10,
			FilesChanged: 20,
			Additions:    500,
			Deletions:    100,
			Contributors: 2,
		},
	}

	result := FormatReleaseNotes(release)

	// Check title
	if !strings.Contains(result, "# Release v2.0.0") {
		t.Error("missing release title")
	}

	// Check date
	if !strings.Contains(result, "June 15, 2024") {
		t.Error("missing formatted date")
	}

	// Check highlights
	if !strings.Contains(result, "## Highlights") {
		t.Error("missing Highlights section")
	}

	// Check breaking changes warning
	if !strings.Contains(result, "## Breaking Changes") {
		t.Error("missing Breaking Changes section")
	}
	if !strings.Contains(result, "Warning") {
		t.Error("missing warning in breaking changes")
	}

	// Check contributors
	if !strings.Contains(result, "## Contributors") {
		t.Error("missing Contributors section")
	}
	if !strings.Contains(result, "@alice") {
		t.Error("missing contributor alice")
	}
	if !strings.Contains(result, "@bob") {
		t.Error("missing contributor bob")
	}

	// Check commit hash in parentheses
	if !strings.Contains(result, "`abc1234`") {
		t.Error("missing commit hash reference")
	}

	// Check author attribution
	if !strings.Contains(result, "@alice") {
		t.Error("missing author attribution")
	}

	// Check stats footer
	if !strings.Contains(result, "Full Changelog") {
		t.Error("missing Full Changelog footer")
	}
}

func TestFormatReleaseNotes_NoBreaking(t *testing.T) {
	release := &Release{
		Version: "1.1.0",
		Date:    time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Changes: []ChangeEntry{
			{Type: "feat", Description: "new thing"},
		},
		BreakingChanges: nil,
		Contributors:    []string{"dev"},
		Stats:           ReleaseStats{Commits: 1, FilesChanged: 1, Additions: 10, Deletions: 0, Contributors: 1},
	}

	result := FormatReleaseNotes(release)
	if strings.Contains(result, "## Breaking Changes") {
		t.Error("should not have Breaking Changes section when there are none")
	}
}

func TestDetectCurrentVersion_PackageJSON(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo without tags
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	// Create package.json
	pkgContent := `{"name": "test-pkg", "version": "2.1.0"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Need at least one commit for git describe to work (even if it fails)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	version, err := rm.DetectCurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != "2.1.0" {
		t.Errorf("got %s, want 2.1.0", version)
	}
}

func TestDetectCurrentVersion_GitTag(t *testing.T) {
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "tag", "v1.5.3")

	rm := NewReleaseManager(dir)
	version, err := rm.DetectCurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.5.3" {
		t.Errorf("got %s, want 1.5.3", version)
	}
}

func TestDetectCurrentVersion_CargoToml(t *testing.T) {
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	cargoContent := `[package]
name = "myapp"
version = "3.2.1"
`
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargoContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.rs"), []byte("fn main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	version, err := rm.DetectCurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != "3.2.1" {
		t.Errorf("got %s, want 3.2.1", version)
	}
}

func TestDetectCurrentVersion_Default(t *testing.T) {
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	version, err := rm.DetectCurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != "0.0.0" {
		t.Errorf("got %s, want 0.0.0 (default)", version)
	}
}

func TestGatherChanges_TempGitRepo(t *testing.T) {
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "TestUser")

	// Initial commit and tag
	writeFile(t, dir, "file.txt", "v1")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: initial commit")
	runGit(t, dir, "tag", "v1.0.0")

	// Add commits after the tag
	writeFile(t, dir, "file.txt", "v2")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat(core): add new parser")

	writeFile(t, dir, "file.txt", "v3")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "fix(ui): resolve crash on startup")

	writeFile(t, dir, "file.txt", "v4")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat(api)!: redesign endpoints")

	rm := NewReleaseManager(dir)
	changes, err := rm.GatherChanges("v1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	if len(changes) != 3 {
		t.Fatalf("got %d changes, want 3", len(changes))
	}

	// Check that we have the expected types (order is newest first in git log)
	typeCount := map[string]int{}
	var hasBreaking bool
	for _, c := range changes {
		typeCount[c.Type]++
		if c.Breaking {
			hasBreaking = true
		}
		if c.Author != "TestUser" {
			t.Errorf("expected author TestUser, got %s", c.Author)
		}
	}

	if typeCount["feat"] != 2 {
		t.Errorf("expected 2 feat, got %d", typeCount["feat"])
	}
	if typeCount["fix"] != 1 {
		t.Errorf("expected 1 fix, got %d", typeCount["fix"])
	}
	if !hasBreaking {
		t.Error("expected at least one breaking change")
	}
}

func TestGatherChanges_NonConventionalFallback(t *testing.T) {
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "TestUser")

	writeFile(t, dir, "file.txt", "v1")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Initial commit")
	runGit(t, dir, "tag", "v0.1.0")

	// Non-conventional commits
	writeFile(t, dir, "file.txt", "v2")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Add new authentication system")

	writeFile(t, dir, "file.txt", "v3")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Fix crash when loading config")

	writeFile(t, dir, "file.txt", "v4")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Update documentation for API")

	rm := NewReleaseManager(dir)
	changes, err := rm.GatherChanges("v0.1.0")
	if err != nil {
		t.Fatal(err)
	}

	if len(changes) != 3 {
		t.Fatalf("got %d changes, want 3", len(changes))
	}

	// Check fallback classification
	typeFound := map[string]bool{}
	for _, c := range changes {
		typeFound[c.Type] = true
	}

	if !typeFound["feat"] {
		t.Error("expected 'Add new...' to be classified as feat")
	}
	if !typeFound["fix"] {
		t.Error("expected 'Fix crash...' to be classified as fix")
	}
	if !typeFound["docs"] {
		t.Error("expected 'Update documentation...' to be classified as docs")
	}
}

func TestValidateRelease_Valid(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "file.txt", "content")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	release := &Release{
		Version: "1.0.0",
		Date:    time.Now(),
		Changes: []ChangeEntry{{Type: "feat", Description: "something"}},
	}

	issues := rm.ValidateRelease(release)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got: %v", issues)
	}
}

func TestValidateRelease_NoChanges(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "file.txt", "content")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	release := &Release{
		Version: "1.0.0",
		Date:    time.Now(),
		Changes: nil,
	}

	issues := rm.ValidateRelease(release)
	if !containsIssue(issues, "no changes") {
		t.Error("expected 'no changes' issue")
	}
}

func TestValidateRelease_InvalidVersion(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "file.txt", "content")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	release := &Release{
		Version: "not-a-version",
		Date:    time.Now(),
		Changes: []ChangeEntry{{Type: "fix", Description: "something"}},
	}

	issues := rm.ValidateRelease(release)
	if !containsIssue(issues, "invalid version") {
		t.Errorf("expected 'invalid version' issue, got: %v", issues)
	}
}

func TestValidateRelease_UncommittedChanges(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "file.txt", "content")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// Create uncommitted change
	writeFile(t, dir, "dirty.txt", "uncommitted")

	rm := NewReleaseManager(dir)
	release := &Release{
		Version: "1.0.0",
		Date:    time.Now(),
		Changes: []ChangeEntry{{Type: "fix", Description: "something"}},
	}

	issues := rm.ValidateRelease(release)
	if !containsIssue(issues, "uncommitted") {
		t.Errorf("expected 'uncommitted' issue, got: %v", issues)
	}
}

func TestValidateRelease_Nil(t *testing.T) {
	dir := t.TempDir()
	rm := NewReleaseManager(dir)
	issues := rm.ValidateRelease(nil)
	if len(issues) != 1 || issues[0] != "release is nil" {
		t.Errorf("expected 'release is nil', got: %v", issues)
	}
}

func TestValidateRelease_EmptyVersion(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "file.txt", "x")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	rm := NewReleaseManager(dir)
	release := &Release{
		Version: "",
		Date:    time.Now(),
		Changes: []ChangeEntry{{Type: "fix", Description: "x"}},
	}

	issues := rm.ValidateRelease(release)
	if !containsIssue(issues, "version is empty") {
		t.Errorf("expected 'version is empty' issue, got: %v", issues)
	}
}

func TestUpdateVersionFile_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "package.json")
	content := `{
  "name": "myapp",
  "version": "1.0.0",
  "description": "test"
}`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateVersionFile("2.0.0", filePath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version": "2.0.0"`) {
		t.Errorf("version not updated, got:\n%s", string(data))
	}
}

func TestUpdateVersionFile_GoFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "version.go")
	content := `package main

const Version = "1.2.3"
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateVersionFile("1.3.0", filePath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `Version = "1.3.0"`) {
		t.Errorf("version not updated, got:\n%s", string(data))
	}
}

func TestUpdateVersionFile_CargoToml(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "Cargo.toml")
	content := `[package]
name = "myapp"
version = "0.5.0"
edition = "2021"
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateVersionFile("0.6.0", filePath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `version = "0.6.0"`) {
		t.Errorf("version not updated, got:\n%s", string(data))
	}
}

func TestUpdateVersionFile_NoPattern(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "random.txt")
	content := "no version here\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := UpdateVersionFile("1.0.0", filePath)
	if err == nil {
		t.Error("expected error when no version pattern found")
	}
	if !strings.Contains(err.Error(), "no version pattern found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{12345, "12,345"},
		{1234567, "1,234,567"},
	}

	for _, tt := range tests {
		got := formatNumber(tt.input)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsValidSemver(t *testing.T) {
	valid := []string{"1.0.0", "0.0.1", "10.20.30", "v1.2.3"}
	for _, v := range valid {
		if !isValidSemver(v) {
			t.Errorf("expected %q to be valid semver", v)
		}
	}

	invalid := []string{"", "1.0", "1.0.0.0", "abc", "1.a.0", "v"}
	for _, v := range invalid {
		if isValidSemver(v) {
			t.Errorf("expected %q to be invalid semver", v)
		}
	}
}

func TestClassifyNonConventional(t *testing.T) {
	tests := []struct {
		msg      string
		wantType string
	}{
		{"Add new authentication system", "feat"},
		{"Fix crash when loading config", "fix"},
		{"Update documentation for API", "docs"},
		{"Refactor database layer", "refactor"},
		{"Improve performance of query engine", "perf"},
		{"Add tests for parser", "test"},
		{"Bump dependency versions", "chore"},
		{"Implement caching layer", "feat"},
		{"Bug in login flow resolved", "fix"},
		{"Optimize memory usage", "perf"},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			entry := classifyNonConventional(tt.msg)
			if entry.Type != tt.wantType {
				t.Errorf("classifyNonConventional(%q) type = %q, want %q", tt.msg, entry.Type, tt.wantType)
			}
		})
	}
}

// Helper functions

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2024-01-01T00:00:00Z")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, output)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsIssue(issues []string, substr string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, substr) {
			return true
		}
	}
	return false
}
