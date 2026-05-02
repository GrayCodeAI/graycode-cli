package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/routing"
)

func TestNewCascadeRouter(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	if cr == nil {
		t.Fatal("expected non-nil router")
	}
	if !cr.Enabled {
		t.Error("expected router to be enabled by default")
	}
	if cr.FrugalMode {
		t.Error("expected frugal mode to be off by default")
	}
	if cr.DefaultModel != "claude-sonnet-4-20250514" {
		t.Errorf("expected default model claude-sonnet-4-20250514, got %q", cr.DefaultModel)
	}
}

func TestClassifyPrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		// Debug signals
		{"fix bug", "fix the null pointer bug in handler.go", "debug"},
		{"error message", "I'm getting an error when running tests", "debug"},
		{"debug keyword", "debug this function please", "debug"},
		{"crash report", "the server is crashing on startup", "debug"},
		{"panic", "I see a panic in the goroutine", "debug"},

		// Refactor signals
		{"refactor", "refactor the database layer to use interfaces", "refactor"},
		{"rename", "rename the variable from x to count", "refactor"},
		{"simplify", "simplify this function", "refactor"},
		{"restructure", "restructure the package layout", "refactor"},
		{"extract", "extract this logic into a helper function", "refactor"},

		// Review signals
		{"review", "review my pull request changes", "review"},
		{"audit", "audit this code for security issues", "review"},
		{"feedback", "give me feedback on this implementation", "review"},
		{"critique", "critique this design approach", "review"},

		// Generation signals
		{"implement", "implement a binary search function", "generation"},
		{"create", "create a new REST API endpoint", "generation"},
		{"write code", "write a test for the parser", "generation"},
		{"build feature", "build a caching layer for the DB queries", "generation"},
		{"generate", "generate Go structs from this JSON schema", "generation"},
		{"scaffold", "scaffold a new microservice", "generation"},

		// Chat signals
		{"explain", "explain how goroutines work", "chat"},
		{"what is", "what is a closure in Go?", "chat"},
		{"how does", "how does the GC work?", "chat"},
		{"why", "why is this approach better?", "chat"},
		{"describe", "describe the architecture of this system", "chat"},

		// Simple signals (short, no strong keywords)
		{"short question", "hello", "simple"},
		{"yes no", "yes", "simple"},
		{"ok", "sounds good", "simple"},

		// Default to chat for longer unclassified prompts
		{"long unclassified", "I was thinking about the overall approach to the project and wanted to discuss the roadmap going forward", "chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyPrompt(tt.prompt)
			if got != tt.expected {
				t.Errorf("classifyPrompt(%q) = %q, want %q", tt.prompt, got, tt.expected)
			}
		})
	}
}

func TestSelectModel_UserOverride(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	// User override should always win, regardless of classification.
	selected := cr.SelectModel("fix the bug", "claude-sonnet-4-20250514", "gpt-4o")
	if selected != "gpt-4o" {
		t.Errorf("user override should win, got %q", selected)
	}

	// Verify the decision was recorded with the right reason.
	decs := cr.Decisions()
	if len(decs) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decs))
	}
	if decs[0].TaskType != "override" {
		t.Errorf("expected task type 'override', got %q", decs[0].TaskType)
	}
	if decs[0].SelectedModel != "gpt-4o" {
		t.Errorf("expected selected model 'gpt-4o', got %q", decs[0].SelectedModel)
	}
}

func TestSelectModel_Disabled(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	cr.Enabled = false

	// When disabled, always return the current model.
	selected := cr.SelectModel("implement a full web framework", "claude-haiku-3-20250307", "")
	if selected != "claude-haiku-3-20250307" {
		t.Errorf("disabled router should pass through current model, got %q", selected)
	}
}

func TestSelectModel_DebugRouting(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	// Debug tasks should route to the reviewer (mid-tier / sonnet).
	selected := cr.SelectModel("fix the segfault in main.go", "claude-sonnet-4-20250514", "")
	if selected != "claude-sonnet-4-20250514" {
		t.Errorf("debug should route to sonnet/reviewer, got %q", selected)
	}
}

func TestSelectModel_GenerationRouting(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	// Generation tasks should route to the planner (expensive tier / opus).
	selected := cr.SelectModel("implement a distributed consensus algorithm", "claude-sonnet-4-20250514", "")
	if selected != "claude-opus-4-20250514" {
		t.Errorf("generation should route to opus/planner, got %q", selected)
	}
}

func TestSelectModel_SimpleRouting(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	cr.FrugalMode = true // enable frugal so downgrades are allowed

	// Simple tasks should route to the commit model (cheap tier / haiku).
	selected := cr.SelectModel("yes", "claude-sonnet-4-20250514", "")
	if selected != "claude-haiku-3-20250307" {
		t.Errorf("simple task (frugal) should route to haiku/commit, got %q", selected)
	}
}

func TestSelectModel_NoDowngradeWithoutFrugal(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	cr.FrugalMode = false

	// Without frugal mode, a simple prompt should NOT downgrade from sonnet.
	selected := cr.SelectModel("ok", "claude-sonnet-4-20250514", "")
	if selected != "claude-sonnet-4-20250514" {
		t.Errorf("without frugal, should not downgrade from sonnet, got %q", selected)
	}
}

func TestSelectModel_FrugalDowngradesChatAndReview(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	cr.FrugalMode = true

	// Frugal mode should downgrade chat from mid to cheap.
	selected := cr.SelectModel("explain what a goroutine is", "claude-opus-4-20250514", "")
	if selected != "claude-haiku-3-20250307" {
		t.Errorf("frugal should downgrade chat to haiku, got %q", selected)
	}

	// Frugal mode should downgrade review from mid to cheap.
	selected = cr.SelectModel("review this code for issues", "claude-opus-4-20250514", "")
	if selected != "claude-haiku-3-20250307" {
		t.Errorf("frugal should downgrade review to haiku, got %q", selected)
	}
}

func TestSelectModel_FrugalCapsGeneration(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	cr.FrugalMode = true

	// Frugal mode should cap generation at mid-tier (sonnet), not opus.
	selected := cr.SelectModel("implement a new parser", "claude-haiku-3-20250307", "")
	if selected != "claude-sonnet-4-20250514" {
		t.Errorf("frugal should cap generation at sonnet/coder, got %q", selected)
	}
}

func TestTierOf(t *testing.T) {
	tests := []struct {
		model string
		tier  ModelTier
	}{
		{"claude-haiku-3-20250307", TierCheap},
		{"gpt-4o-mini", TierCheap},
		{"gpt-3.5-turbo", TierCheap},
		{"gemini-2.5-flash", TierCheap},
		{"deepseek-chat", TierCheap},
		{"mistral-small", TierCheap},
		{"claude-sonnet-4-20250514", TierMid},
		{"gpt-4o", TierMid},
		{"gpt-4-turbo", TierMid},
		{"claude-opus-4-20250514", TierExpensive},
		{"unknown-model-xyz", TierMid},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := tierOf(tt.model)
			if got != tt.tier {
				t.Errorf("tierOf(%q) = %d, want %d", tt.model, got, tt.tier)
			}
		})
	}
}

func TestDecisions_Tracking(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	if cr.DecisionCount() != 0 {
		t.Fatalf("expected 0 decisions initially, got %d", cr.DecisionCount())
	}

	cr.SelectModel("fix the bug", "claude-sonnet-4-20250514", "")
	cr.SelectModel("implement a parser", "claude-sonnet-4-20250514", "")
	cr.SelectModel("hello", "claude-sonnet-4-20250514", "gpt-4o")

	if cr.DecisionCount() != 3 {
		t.Fatalf("expected 3 decisions, got %d", cr.DecisionCount())
	}

	decs := cr.Decisions()
	if len(decs) != 3 {
		t.Fatalf("expected 3 decisions in snapshot, got %d", len(decs))
	}

	// First: debug classification
	if decs[0].TaskType != "debug" {
		t.Errorf("decision[0] task type = %q, want 'debug'", decs[0].TaskType)
	}
	// Second: generation classification
	if decs[1].TaskType != "generation" {
		t.Errorf("decision[1] task type = %q, want 'generation'", decs[1].TaskType)
	}
	// Third: user override
	if decs[2].TaskType != "override" {
		t.Errorf("decision[2] task type = %q, want 'override'", decs[2].TaskType)
	}

	// Verify timestamps are populated
	for i, d := range decs {
		if d.Timestamp.IsZero() {
			t.Errorf("decision[%d] has zero timestamp", i)
		}
	}
}

func TestSavings(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	// No decisions yet -- zero savings.
	if s := cr.Savings(); s != 0 {
		t.Errorf("expected 0 savings initially, got %f", s)
	}

	// Record a decision where the model was downgraded.
	// Use model names that are in the engine's local pricing fallback map
	// (gpt-4 @ $30/M vs gpt-4o-mini @ $0.15/M) so the price difference
	// is resolvable even without the eyrie catalog loaded.
	cr.record("gpt-4", "gpt-4o-mini", "simple", "test")

	savings := cr.Savings()
	if savings <= 0 {
		t.Errorf("expected positive savings for downgrade, got %f", savings)
	}
}

func TestSummary(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	// Empty summary
	summary := cr.Summary()
	if summary == "" {
		t.Error("expected non-empty summary even with no decisions")
	}

	// Add some decisions
	cr.SelectModel("fix the bug", "claude-sonnet-4-20250514", "")
	cr.SelectModel("implement a parser", "claude-sonnet-4-20250514", "")

	summary = cr.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	// Should mention decision count
	if !promptContainsAny(summary, "2 decisions") {
		t.Errorf("summary should mention decision count, got: %s", summary)
	}
}

func TestPromptContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		substrs  []string
		expected bool
	}{
		{"fix the bug", []string{"fix", "error"}, true},
		{"hello world", []string{"fix", "error"}, false},
		{"this has an error in it", []string{"fix", "error"}, true},
		{"", []string{"anything"}, false},
		{"something", []string{}, false},
	}

	for _, tt := range tests {
		got := promptContainsAny(tt.s, tt.substrs...)
		if got != tt.expected {
			t.Errorf("promptContainsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, got, tt.expected)
		}
	}
}

func TestSelectModel_EmptyRoles(t *testing.T) {
	// With empty roles, the router should fall back to canonical tier names.
	cr := NewCascadeRouter("claude-sonnet-4-20250514", routing.ModelRoles{})
	cr.FrugalMode = true

	// Simple prompt with empty roles should attempt to select a cheaper model.
	selected := cr.SelectModel("ok", "claude-opus-4-20250514", "")
	if selected == "" {
		t.Error("empty roles + simple task should still return a model")
	}

	// Generation prompt should return a non-empty model.
	selected = cr.SelectModel("implement a compiler", "claude-haiku-3-20250307", "")
	if selected == "" {
		t.Error("empty roles + generation should still return a model")
	}
}

func TestSelectModel_EmptyOverrideIgnored(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)

	// Whitespace-only override should be ignored (not treated as user choice).
	selected := cr.SelectModel("fix the crash", "claude-sonnet-4-20250514", "   ")
	if selected != "claude-sonnet-4-20250514" {
		t.Errorf("whitespace override should be ignored, got %q", selected)
	}

	decs := cr.Decisions()
	if len(decs) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decs))
	}
	if decs[0].TaskType == "override" {
		t.Error("whitespace-only should not be recorded as override")
	}
}

func TestSelectModel_UpgradeAllowed(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-3-20250307",
	}
	cr := NewCascadeRouter("claude-sonnet-4-20250514", roles)
	cr.FrugalMode = false

	// Even without frugal mode, upgrades should be allowed.
	// Starting from haiku, a generation prompt should upgrade to opus.
	selected := cr.SelectModel("implement a full distributed system", "claude-haiku-3-20250307", "")
	if selected != "claude-opus-4-20250514" {
		t.Errorf("should upgrade from haiku to opus for generation, got %q", selected)
	}
}
