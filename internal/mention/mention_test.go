package mention

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMentions_NoMentions(t *testing.T) {
	result := ParseMentions("just a normal prompt", "/project")
	if result.CleanInput != "just a normal prompt" {
		t.Errorf("unexpected clean input: %q", result.CleanInput)
	}
	if len(result.MentionedFiles) != 0 {
		t.Errorf("expected no mentioned files, got %d", len(result.MentionedFiles))
	}
}

func TestParseMentions_SingleMention(t *testing.T) {
	result := ParseMentions("fix the bug in @src/main.go", "/project")
	if len(result.RawMentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(result.RawMentions))
	}
	if result.RawMentions[0] != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", result.RawMentions[0])
	}
	if result.CleanInput != "fix the bug in" {
		t.Errorf("unexpected clean input: %q", result.CleanInput)
	}
}

func TestParseMentions_MultipleMentions(t *testing.T) {
	result := ParseMentions("compare @file1.go and @pkg/file2.go", "/project")
	if len(result.RawMentions) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(result.RawMentions))
	}
	if result.RawMentions[0] != "file1.go" {
		t.Errorf("first mention: expected 'file1.go', got %q", result.RawMentions[0])
	}
	if result.RawMentions[1] != "pkg/file2.go" {
		t.Errorf("second mention: expected 'pkg/file2.go', got %q", result.RawMentions[1])
	}
}

func TestParseMentions_QuotedPaths(t *testing.T) {
	result := ParseMentions(`check @"path with spaces/file.go" please`, "/project")
	if len(result.RawMentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(result.RawMentions))
	}
	if result.RawMentions[0] != "path with spaces/file.go" {
		t.Errorf("expected quoted path, got %q", result.RawMentions[0])
	}
}

func TestIsMention(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"@file.go", true},
		{"@src/main.go", true},
		{"@", false},
		{"file.go", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsMention(tt.input)
		if got != tt.want {
			t.Errorf("IsMention(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractPartial(t *testing.T) {
	tests := []struct {
		input     string
		cursorPos int
		want      string
	}{
		{"fix @src/m", 10, "src/m"},
		{"fix @", 5, ""},
		{"fix @src/main.go done", 16, "src/main.go"},
		{"no mention", 10, ""},
	}

	for _, tt := range tests {
		got := ExtractPartial(tt.input, tt.cursorPos)
		if got != tt.want {
			t.Errorf("ExtractPartial(%q, %d) = %q, want %q", tt.input, tt.cursorPos, got, tt.want)
		}
	}
}

func TestFuzzyMatch(t *testing.T) {
	// Create a temporary project structure.
	tmp := t.TempDir()
	dirs := []string{"src", "src/utils", "pkg", "cmd"}
	files := []string{
		"src/main.go",
		"src/utils/helper.go",
		"src/utils/math.go",
		"pkg/config.go",
		"cmd/root.go",
	}

	for _, d := range dirs {
		os.MkdirAll(filepath.Join(tmp, d), 0o755)
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(tmp, f), []byte("package main"), 0o644)
	}

	// Test prefix match.
	matches := FuzzyMatch("src/m", tmp, 5)
	if len(matches) == 0 {
		t.Fatal("expected matches for 'src/m'")
	}
	found := false
	for _, m := range matches {
		if m == "src/main.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected src/main.go in matches, got %v", matches)
	}

	// Test filename match.
	matches = FuzzyMatch("helper", tmp, 5)
	if len(matches) == 0 {
		t.Fatal("expected matches for 'helper'")
	}

	// Test no match.
	matches = FuzzyMatch("zzzzzzz", tmp, 5)
	if len(matches) != 0 {
		t.Errorf("expected no matches for 'zzzzzzz', got %v", matches)
	}
}

func TestFuzzyScore(t *testing.T) {
	tests := []struct {
		partial   string
		candidate string
		wantPos   bool // true if score > 0
	}{
		{"main", "src/main.go", true},
		{"src/m", "src/main.go", true},
		{"zzz", "src/main.go", false},
		{"", "src/main.go", false},
		{"config", "pkg/config.go", true},
	}

	for _, tt := range tests {
		score := fuzzyScore(tt.partial, tt.candidate)
		if tt.wantPos && score <= 0 {
			t.Errorf("fuzzyScore(%q, %q) = %d, want > 0", tt.partial, tt.candidate, score)
		}
		if !tt.wantPos && score > 0 {
			t.Errorf("fuzzyScore(%q, %q) = %d, want 0", tt.partial, tt.candidate, score)
		}
	}
}
