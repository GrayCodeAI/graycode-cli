package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFeedbackRateAndGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.json")
	fs := NewFeedbackStoreAt(path)

	if err := fs.Rate("api-review", 4, "very useful"); err != nil {
		t.Fatalf("Rate: %v", err)
	}

	r, ok := fs.Get("api-review")
	if !ok {
		t.Fatal("expected rating to exist")
	}
	if r.Rating != 4 {
		t.Errorf("expected 4, got %d", r.Rating)
	}
	if r.Comment != "very useful" {
		t.Errorf("expected 'very useful', got %q", r.Comment)
	}
}

func TestFeedbackUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.json")
	fs := NewFeedbackStoreAt(path)

	fs.Rate("test", 3, "ok")
	fs.Rate("test", 5, "great now")

	r, _ := fs.Get("test")
	if r.Rating != 5 {
		t.Errorf("expected updated rating 5, got %d", r.Rating)
	}
	if r.Comment != "great now" {
		t.Errorf("expected updated comment")
	}

	// Should still be only 1 entry.
	all := fs.List()
	if len(all) != 1 {
		t.Errorf("expected 1 rating, got %d", len(all))
	}
}

func TestFeedbackInvalidRating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.json")
	fs := NewFeedbackStoreAt(path)

	if err := fs.Rate("test", 0, ""); err == nil {
		t.Error("expected error for rating 0")
	}
	if err := fs.Rate("test", 6, ""); err == nil {
		t.Error("expected error for rating 6")
	}
}

func TestFeedbackGetMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.json")
	fs := NewFeedbackStoreAt(path)

	_, ok := fs.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestFormatRating(t *testing.T) {
	tests := []struct {
		rating int
		want   string
	}{
		{1, "★☆☆☆☆"},
		{3, "★★★☆☆"},
		{5, "★★★★★"},
	}
	for _, tt := range tests {
		got := FormatRating(tt.rating)
		if got != tt.want {
			t.Errorf("FormatRating(%d) = %q, want %q", tt.rating, got, tt.want)
		}
	}
}

func TestBuildLearnPrompt(t *testing.T) {
	ctx := LearnContext{
		Signals:   []ProjectSignal{{Category: "language", Name: "go"}},
		Installed: []SmartSkill{{Name: "api-review", Description: "Reviews APIs"}},
		Registry:  []SkillEntry{{Name: "go-patterns", Category: "engineering", Description: "Go patterns", Installs: 100}},
	}

	prompt := BuildLearnPrompt(ctx)
	if !strings.Contains(prompt, "language: go") {
		t.Error("expected project signal in prompt")
	}
	if !strings.Contains(prompt, "api-review") {
		t.Error("expected installed skill in prompt")
	}
	if !strings.Contains(prompt, "go-patterns") {
		t.Error("expected registry skill in prompt")
	}
	if !strings.Contains(prompt, "Score each community skill 0-100") {
		t.Error("expected scoring instructions")
	}
}

func TestBuildLearnPromptWithSourceInfo(t *testing.T) {
	ctx := LearnContext{
		SourceInfo: "### go.mod\n```\nmodule test\n```\n",
	}
	prompt := BuildLearnPrompt(ctx)
	if !strings.Contains(prompt, "Source File Analysis") {
		t.Error("expected source file section")
	}
	if !strings.Contains(prompt, "go.mod") {
		t.Error("expected go.mod content")
	}
}

func TestGatherDeepSourceInfo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project"), 0o644)

	info := GatherDeepSourceInfo(dir)
	if !strings.Contains(info, "go.mod") {
		t.Error("expected go.mod in source info")
	}
	if !strings.Contains(info, "README.md") {
		t.Error("expected README.md in source info")
	}
}

func TestGatherDeepSourceInfoEmpty(t *testing.T) {
	dir := t.TempDir()
	info := GatherDeepSourceInfo(dir)
	if !strings.Contains(info, "No key source files") {
		t.Error("expected empty message")
	}
}

func TestGatherDeepSourceInfoTruncation(t *testing.T) {
	dir := t.TempDir()
	// Create a file larger than 2000 chars.
	big := strings.Repeat("x", 3000)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte(big), 0o644)

	info := GatherDeepSourceInfo(dir)
	if !strings.Contains(info, "truncated") {
		t.Error("expected truncation marker")
	}
}

func TestFormatLearnSummary(t *testing.T) {
	ctx := LearnContext{
		Signals:   []ProjectSignal{{Category: "language", Name: "go"}},
		Installed: []SmartSkill{{Name: "test"}},
		Registry:  []SkillEntry{{Name: "a"}, {Name: "b"}},
	}

	out := FormatLearnSummary(ctx, false)
	if !strings.Contains(out, "/learn") {
		t.Error("expected /learn in summary")
	}
	if !strings.Contains(out, "Detected: go") {
		t.Error("expected detected signals")
	}

	out = FormatLearnSummary(ctx, true)
	if !strings.Contains(out, "/learn deep") {
		t.Error("expected /learn deep in summary")
	}
}
