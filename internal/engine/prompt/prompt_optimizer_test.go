package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptOptimizer_RegisterAndGet(t *testing.T) {
	po := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: filepath.Join(t.TempDir(), "params.json")}
	po.Register("system", "You are a helpful assistant")
	got := po.Get("system")
	if got != "You are a helpful assistant" {
		t.Errorf("got %q", got)
	}
}

func TestPromptOptimizer_RecordSuccess(t *testing.T) {
	po := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: filepath.Join(t.TempDir(), "params.json")}
	po.Register("test", "value")
	po.RecordSuccess("test")
	po.RecordSuccess("test")
	if po.Parameters["test"].Score <= 0.5 {
		t.Errorf("score should increase after success, got %f", po.Parameters["test"].Score)
	}
}

func TestPromptOptimizer_RecordFailure(t *testing.T) {
	po := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: filepath.Join(t.TempDir(), "params.json")}
	po.Register("test", "value")
	po.RecordFailure("test", "bad output")
	po.RecordFailure("test", "still bad")
	if po.Parameters["test"].Score >= 0.5 {
		t.Errorf("score should decrease after failure, got %f", po.Parameters["test"].Score)
	}
}

func TestPromptOptimizer_NeedsOptimization(t *testing.T) {
	po := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: filepath.Join(t.TempDir(), "params.json")}
	po.Register("good", "works well")
	po.Register("bad", "doesn't work")
	po.Parameters["good"].Score = 0.9
	po.Parameters["bad"].Score = 0.2

	weak := po.NeedsOptimization(0.5)
	if len(weak) != 1 {
		t.Errorf("expected 1 weak param, got %d", len(weak))
	}
	if weak[0].Name != "bad" {
		t.Errorf("expected 'bad', got %q", weak[0].Name)
	}
}

func TestPromptOptimizer_ApplyGradient(t *testing.T) {
	po := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: filepath.Join(t.TempDir(), "params.json")}
	po.Register("prompt", "old value")
	po.ApplyGradient("prompt", "new improved value", PromptGradient{Parameter: "prompt", Direction: "be more specific"})
	if po.Get("prompt") != "new improved value" {
		t.Errorf("got %q", po.Get("prompt"))
	}
	if po.Parameters["prompt"].Version != 2 {
		t.Errorf("version = %d, want 2", po.Parameters["prompt"].Version)
	}
}

func TestPromptOptimizer_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	po1 := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: path}
	po1.Register("key", "value")
	po1.save()

	po2 := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: path}
	po2.load()
	if po2.Get("key") != "value" {
		t.Errorf("persistence failed, got %q", po2.Get("key"))
	}
}

func TestFewShotSelector_Select(t *testing.T) {
	fs := &PromptFewShotSelector{
		Examples: []PromptExample{
			{Input: "fix the nil pointer in auth.go", Output: "added nil check", Score: 0.9, Category: "bug-fix"},
			{Input: "add pagination to users API", Output: "added limit/offset", Score: 0.8, Category: "feature"},
			{Input: "fix race condition in cache", Output: "added mutex", Score: 0.7, Category: "bug-fix"},
		},
	}

	selected := fs.Select("fix the nil pointer dereference in parser.go", 2)
	if len(selected) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(selected))
	}
	if selected[0].Input != "fix the nil pointer in auth.go" {
		t.Errorf("expected nil pointer example first, got %q", selected[0].Input)
	}
}

func TestFewShotSelector_Empty(t *testing.T) {
	fs := &PromptFewShotSelector{}
	selected := fs.Select("anything", 3)
	if len(selected) != 0 {
		t.Error("expected empty for no examples")
	}
}

func TestComputeGradientPrompt(t *testing.T) {
	prompt := ComputeGradientPrompt("system_prompt", "You are helpful", "Too verbose, wastes tokens", nil)
	if !strings.Contains(prompt, "system_prompt") {
		t.Error("expected param name")
	}
	if !strings.Contains(prompt, "Too verbose") {
		t.Error("expected feedback")
	}
}

func TestFormatPromptExamples(t *testing.T) {
	examples := []PromptExample{{Input: "fix bug", Output: "fixed"}}
	out := FormatPromptExamples(examples)
	if !strings.Contains(out, "fix bug") {
		t.Error("expected example in output")
	}
}

func TestPromptOptimizer_SaveLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	po := &PromptOptimizer{Parameters: make(map[string]*PromptParameter), Path: path}
	po.Register("x", "y")
	po.save()
	if _, err := os.Stat(path); err != nil {
		t.Error("file not created")
	}
}
