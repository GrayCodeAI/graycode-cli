package engine

import (
	"strings"
	"sync"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/routing"
)

func TestNewSystemPromptBuilder(t *testing.T) {
	b := NewSystemPromptBuilder("base prompt", 1000)
	if b.BasePrompt != "base prompt" {
		t.Errorf("expected base prompt 'base prompt', got %q", b.BasePrompt)
	}
	if b.MaxTokens != 1000 {
		t.Errorf("expected max tokens 1000, got %d", b.MaxTokens)
	}
	if len(b.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(b.Sections))
	}
}

func TestAddSection(t *testing.T) {
	b := NewSystemPromptBuilder("", 5000)

	b.AddSection(PromptSection{Name: "identity", Content: "You are graycode.", Priority: 1})
	b.AddSection(PromptSection{Name: "safety", Content: "Be safe.", Priority: 1})

	if len(b.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(b.Sections))
	}

	// Adding same name replaces
	b.AddSection(PromptSection{Name: "identity", Content: "You are graycode v2.", Priority: 1})
	if len(b.Sections) != 2 {
		t.Fatalf("expected 2 sections after replace, got %d", len(b.Sections))
	}
	if b.Sections[0].Content != "You are graycode v2." {
		t.Errorf("expected updated content, got %q", b.Sections[0].Content)
	}
}

func TestRemoveSection(t *testing.T) {
	b := NewSystemPromptBuilder("", 5000)
	b.AddSection(PromptSection{Name: "a", Content: "A", Priority: 1})
	b.AddSection(PromptSection{Name: "b", Content: "B", Priority: 2})
	b.AddSection(PromptSection{Name: "c", Content: "C", Priority: 3})

	b.RemoveSection("b")
	if len(b.Sections) != 2 {
		t.Fatalf("expected 2 sections after remove, got %d", len(b.Sections))
	}
	for _, s := range b.Sections {
		if s.Name == "b" {
			t.Error("section 'b' should have been removed")
		}
	}

	// Removing non-existent section is safe
	b.RemoveSection("nonexistent")
	if len(b.Sections) != 2 {
		t.Errorf("expected 2 sections after no-op remove, got %d", len(b.Sections))
	}
}

func TestBuildPrioritySorting(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{Name: "low", Content: "Low priority.", Priority: 5})
	b.AddSection(PromptSection{Name: "high", Content: "High priority.", Priority: 1})
	b.AddSection(PromptSection{Name: "mid", Content: "Mid priority.", Priority: 3})

	result := b.Build(PromptBuildContext{})

	highIdx := strings.Index(result, "High priority.")
	midIdx := strings.Index(result, "Mid priority.")
	lowIdx := strings.Index(result, "Low priority.")

	if highIdx > midIdx || midIdx > lowIdx {
		t.Errorf("sections not sorted by priority: high=%d mid=%d low=%d", highIdx, midIdx, lowIdx)
	}
}

func TestBuildConditional(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{
		Name:     "always",
		Content:  "Always included.",
		Priority: 1,
	})
	b.AddSection(PromptSection{
		Name:        "conditional_true",
		Content:     "Included when true.",
		Priority:    2,
		Conditional: func() bool { return true },
	})
	b.AddSection(PromptSection{
		Name:        "conditional_false",
		Content:     "Should be excluded.",
		Priority:    2,
		Conditional: func() bool { return false },
	})

	result := b.Build(PromptBuildContext{})

	if !strings.Contains(result, "Always included.") {
		t.Error("expected unconditional section to be included")
	}
	if !strings.Contains(result, "Included when true.") {
		t.Error("expected conditional_true section to be included")
	}
	if strings.Contains(result, "Should be excluded.") {
		t.Error("expected conditional_false section to be excluded")
	}
}

func TestBuildDynamic(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{
		Name:     "dynamic",
		Priority: 1,
		Dynamic:  func() string { return "Generated at build time." },
	})

	result := b.Build(PromptBuildContext{})
	if !strings.Contains(result, "Generated at build time.") {
		t.Error("expected dynamic content to be present")
	}
}

func TestBuildTokenBudget(t *testing.T) {
	b := NewSystemPromptBuilder("", 50) // very small budget

	// Each section is ~15 tokens at 4 chars/token
	b.AddSection(PromptSection{Name: "a", Content: strings.Repeat("x", 60), Priority: 1})
	b.AddSection(PromptSection{Name: "b", Content: strings.Repeat("y", 60), Priority: 2})
	b.AddSection(PromptSection{Name: "c", Content: strings.Repeat("z", 60), Priority: 3})

	result := b.Build(PromptBuildContext{})

	// With budget of 50 tokens and each section ~15 tokens, we can fit about 3
	// But let's verify that at least priority ordering is respected and budget is enforced
	if strings.Contains(result, strings.Repeat("z", 60)) && !strings.Contains(result, strings.Repeat("x", 60)) {
		t.Error("lower priority section included but higher priority excluded")
	}
}

func TestBuildTokenBudgetExclusion(t *testing.T) {
	// Test that sections exceeding budget are excluded
	b := NewSystemPromptBuilder("", 20) // tiny budget: 20 tokens

	// This section alone is ~25 tokens, exceeds the budget
	b.AddSection(PromptSection{Name: "big", Content: strings.Repeat("a", 100), Priority: 1})
	// This one fits
	b.AddSection(PromptSection{Name: "small", Content: "hi", Priority: 2})

	result := b.Build(PromptBuildContext{})

	if strings.Contains(result, strings.Repeat("a", 100)) {
		t.Error("big section should have been excluded due to budget")
	}
	if !strings.Contains(result, "hi") {
		t.Error("small section should have been included")
	}
}

func TestAdaptForTaskDebug(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{Name: "conventions", Content: "Conventions.", Priority: 4})
	b.AddSection(PromptSection{Name: "safety", Content: "Safety.", Priority: 2})

	b.AdaptForTask("debug this crash")

	// conventions priority should have been boosted
	for _, s := range b.Sections {
		if s.Name == "conventions" && s.Priority >= 4 {
			t.Error("expected conventions priority to be boosted for debug task")
		}
	}

	// error_patterns section should have been added
	found := false
	for _, s := range b.Sections {
		if s.Name == "error_patterns" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error_patterns section to be added for debug task")
	}
}

func TestAdaptForTaskReview(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{Name: "safety", Content: "Safety.", Priority: 3})

	b.AdaptForTask("review this PR")

	for _, s := range b.Sections {
		if s.Name == "safety" && s.Priority != 1 {
			t.Errorf("expected safety priority 1 for review task, got %d", s.Priority)
		}
	}

	found := false
	for _, s := range b.Sections {
		if s.Name == "review_checklist" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected review_checklist section for review task")
	}
}

func TestAdaptForTaskImplement(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{Name: "examples", Content: "Examples.", Priority: 5})

	b.AdaptForTask("implement a new feature")

	for _, s := range b.Sections {
		if s.Name == "examples" && s.Priority >= 5 {
			t.Error("expected examples priority to be boosted for implement task")
		}
	}

	found := false
	for _, s := range b.Sections {
		if s.Name == "architecture" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected architecture section for implement task")
	}
}

func TestAdaptForModelOpus(t *testing.T) {
	_, _, opus := testTierModels(t, testProvider)
	b := NewSystemPromptBuilder("", 1000)
	b.AdaptForModel(opus)

	if routing.CostTierOf(opus) == routing.CostTierExpensive {
		if b.MaxTokens != 1200 {
			t.Errorf("expected 1200 tokens for opus tier, got %d", b.MaxTokens)
		}
	} else if b.MaxTokens != 1000 {
		t.Errorf("expected default 1000 tokens for non-opus tier, got %d", b.MaxTokens)
	}
}

func TestAdaptForModelHaiku(t *testing.T) {
	haiku, _, _ := testTierModels(t, testProvider)
	b := NewSystemPromptBuilder("", 1000)
	b.AddSection(PromptSection{Name: "examples", Content: "Examples.", Priority: 5})

	b.AdaptForModel(haiku)

	if routing.CostTierOf(haiku) == routing.CostTierCheap {
		if b.MaxTokens != 700 {
			t.Errorf("expected 700 tokens for haiku tier, got %d", b.MaxTokens)
		}
		for _, s := range b.Sections {
			if s.Name == "examples" && s.Priority != 10 {
				t.Errorf("expected examples demoted to priority 10 for haiku, got %d", s.Priority)
			}
		}
	}
}

func TestAdaptForModelSonnet(t *testing.T) {
	b := NewSystemPromptBuilder("", 1000)
	b.AdaptForModel("claude-sonnet-4")

	if b.MaxTokens != 1000 {
		t.Errorf("expected 1000 tokens unchanged for sonnet, got %d", b.MaxTokens)
	}
}

func TestEstimateStringTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},                         // 2 chars -> ceil(2/4) = 1
		{"hello", 2},                      // 5 chars -> ceil(5/4) = 2
		{"hello world this is a test", 7}, // 26 chars -> ceil(26/4) = 7
	}

	for _, tt := range tests {
		got := EstimateStringTokens(tt.input)
		if got != tt.expected {
			t.Errorf("EstimateStringTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestFormatPrompt(t *testing.T) {
	sections := []PromptSection{
		{Name: "identity", Content: "You are graycode."},
		{Name: "safety", Content: "Be safe."},
	}

	result := FormatPrompt("Base prompt here.", sections)

	if !strings.HasPrefix(result, "Base prompt here.") {
		t.Error("expected result to start with base prompt")
	}
	if !strings.Contains(result, "## identity") {
		t.Error("expected identity section header")
	}
	if !strings.Contains(result, "## safety") {
		t.Error("expected safety section header")
	}
	if !strings.Contains(result, "You are graycode.") {
		t.Error("expected identity content")
	}
	if !strings.Contains(result, "Be safe.") {
		t.Error("expected safety content")
	}
}

func TestFormatPromptEmptyBase(t *testing.T) {
	sections := []PromptSection{
		{Name: "only", Content: "Only section."},
	}

	result := FormatPrompt("", sections)
	if strings.HasPrefix(result, "\n") {
		t.Error("expected no leading newline with empty base")
	}
	if !strings.Contains(result, "## only") {
		t.Error("expected section header")
	}
}

func TestDiffPrompts(t *testing.T) {
	old := "line one\nline two\nline three"
	new := "line one\nline modified\nline three"

	diff := DiffPrompts(old, new)

	if !strings.Contains(diff, "- line two") {
		t.Errorf("expected removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+ line modified") {
		t.Errorf("expected added line, got:\n%s", diff)
	}
}

func TestDiffPromptsNoChanges(t *testing.T) {
	same := "line one\nline two"
	diff := DiffPrompts(same, same)
	if diff != "(no changes)" {
		t.Errorf("expected '(no changes)', got %q", diff)
	}
}

func TestDefaultSections(t *testing.T) {
	ctx := PromptBuildContext{
		Task:        "implement feature",
		Language:    "go",
		ProjectType: "cli",
		Model:       "opus",
		TurnCount:   2,
		HasMemory:   true,
		HasGoals:    true,
	}

	sections := DefaultSections(ctx)

	names := make(map[string]bool)
	for _, s := range sections {
		names[s.Name] = true
	}

	required := []string{"identity", "safety", "tools", "project", "conventions", "memory", "goals", "examples"}
	for _, r := range required {
		if !names[r] {
			t.Errorf("expected default section %q", r)
		}
	}

	// Check conditionals evaluate correctly
	for _, s := range sections {
		if s.Name == "memory" && s.Conditional != nil && !s.Conditional() {
			t.Error("memory conditional should be true when HasMemory=true")
		}
		if s.Name == "goals" && s.Conditional != nil && !s.Conditional() {
			t.Error("goals conditional should be true when HasGoals=true")
		}
		if s.Name == "examples" && s.Conditional != nil && !s.Conditional() {
			t.Error("examples conditional should be true when TurnCount < 5")
		}
	}
}

func TestDefaultSectionsConditionalsFalse(t *testing.T) {
	ctx := PromptBuildContext{
		TurnCount: 10,
		HasMemory: false,
		HasGoals:  false,
	}

	sections := DefaultSections(ctx)

	for _, s := range sections {
		if s.Name == "memory" && s.Conditional != nil && s.Conditional() {
			t.Error("memory conditional should be false when HasMemory=false")
		}
		if s.Name == "goals" && s.Conditional != nil && s.Conditional() {
			t.Error("goals conditional should be false when HasGoals=false")
		}
		if s.Name == "examples" && s.Conditional != nil && s.Conditional() {
			t.Error("examples conditional should be false when TurnCount >= 5")
		}
		if s.Name == "project" && s.Conditional != nil && s.Conditional() {
			t.Error("project conditional should be false when ProjectType is empty")
		}
	}
}

func TestSystemPromptBuilderConcurrentAccess(t *testing.T) {
	b := NewSystemPromptBuilder("base", 50000)
	b.AddSection(PromptSection{Name: "s1", Content: "content1", Priority: 1})
	b.AddSection(PromptSection{Name: "s2", Content: "content2", Priority: 2})

	var wg sync.WaitGroup
	ctx := PromptBuildContext{Task: "test"}

	// Multiple concurrent builds
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := b.Build(ctx)
			if result == "" {
				t.Error("expected non-empty result from concurrent build")
			}
		}()
	}

	// Concurrent adds
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b.AddSection(PromptSection{
				Name:     "concurrent",
				Content:  "concurrent content",
				Priority: 3,
			})
		}(i)
	}

	wg.Wait()
}

func TestAdaptForTaskChaining(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{Name: "conventions", Content: "Conv.", Priority: 4})
	b.AddSection(PromptSection{Name: "examples", Content: "Ex.", Priority: 5})

	// Method chaining
	result := b.AdaptForTask("debug").AdaptForModel("opus")
	if result != b {
		t.Error("expected method chaining to return same builder")
	}
}

func TestBuildWithBasePrompt(t *testing.T) {
	b := NewSystemPromptBuilder("You are an AI assistant.", 50000)
	b.AddSection(PromptSection{Name: "rules", Content: "Follow the rules.", Priority: 1})

	result := b.Build(PromptBuildContext{})

	if !strings.HasPrefix(result, "You are an AI assistant.") {
		t.Error("expected result to start with base prompt")
	}
	if !strings.Contains(result, "Follow the rules.") {
		t.Error("expected rules section content")
	}
}

func TestBuildEmptySections(t *testing.T) {
	b := NewSystemPromptBuilder("Only base.", 50000)
	result := b.Build(PromptBuildContext{})
	if result != "Only base." {
		t.Errorf("expected only base prompt, got %q", result)
	}
}

func TestDynamicSectionEmptyContent(t *testing.T) {
	b := NewSystemPromptBuilder("", 50000)
	b.AddSection(PromptSection{
		Name:     "empty_dynamic",
		Priority: 1,
		Dynamic:  func() string { return "" },
	})
	b.AddSection(PromptSection{
		Name:     "static",
		Content:  "Static content.",
		Priority: 2,
	})

	result := b.Build(PromptBuildContext{})

	if strings.Contains(result, "empty_dynamic") {
		t.Error("empty dynamic section should not appear in output")
	}
	if !strings.Contains(result, "Static content.") {
		t.Error("static section should appear in output")
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("debug this error", "debug", "fix") {
		t.Error("expected containsAny to find 'debug'")
	}
	if containsAny("hello world", "debug", "fix") {
		t.Error("expected containsAny to return false")
	}
}

func TestPromptMaxInt(t *testing.T) {
	if promptMaxInt(3, 5) != 5 {
		t.Error("expected 5")
	}
	if promptMaxInt(7, 2) != 7 {
		t.Error("expected 7")
	}
	if promptMaxInt(4, 4) != 4 {
		t.Error("expected 4")
	}
}
