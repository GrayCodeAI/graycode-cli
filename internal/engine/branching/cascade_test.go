package branching

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/routing"
)

const testProvider = "anthropic"

// testTierModels loads haiku/sonnet/opus model IDs from eyrie's catalog (not hardcoded).
func testTierModels(t *testing.T, provider string) (haiku, sonnet, opus string) {
	t.Helper()
	haiku = routing.PreferredModelForTier(provider, routing.TierHaiku, "")
	sonnet = routing.PreferredModelForTier(provider, routing.TierSonnet, "")
	opus = routing.PreferredModelForTier(provider, routing.TierOpus, "")
	if haiku == "" || sonnet == "" || opus == "" {
		t.Skipf("no tier models for %q without live catalog (fully dynamic)", provider)
	}
	return haiku, sonnet, opus
}

func testAnthropicRoles(t *testing.T) (roles routing.ModelRoles, defaultModel string) {
	t.Helper()
	haiku, sonnet, opus := testTierModels(t, testProvider)
	return routing.ModelRoles{
		Planner:  opus,
		Coder:    sonnet,
		Reviewer: sonnet,
		Commit:   haiku,
	}, sonnet
}

func TestNewCascadeRouter(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)
	if cr == nil {
		t.Fatal("expected non-nil router")
	}
	if !cr.Enabled {
		t.Error("expected router to be enabled by default")
	}
	if cr.FrugalMode {
		t.Error("expected frugal mode to be off by default")
	}
	if cr.DefaultModel != defaultModel {
		t.Errorf("expected default model %q, got %q", defaultModel, cr.DefaultModel)
	}
}

func TestClassifyPrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		{"fix bug", "fix the null pointer bug in handler.go", "debug"},
		{"error message", "I'm getting an error when running tests", "debug"},
		{"debug keyword", "debug this function please", "debug"},
		{"crash report", "the server is crashing on startup", "debug"},
		{"panic", "I see a panic in the goroutine", "debug"},
		{"refactor", "refactor the database layer to use interfaces", "refactor"},
		{"rename", "rename the variable from x to count", "refactor"},
		{"simplify", "simplify this function", "refactor"},
		{"restructure", "restructure the package layout", "refactor"},
		{"extract", "extract this logic into a helper function", "refactor"},
		{"review", "review my pull request changes", "review"},
		{"audit", "audit this code for security issues", "review"},
		{"feedback", "give me feedback on this implementation", "review"},
		{"critique", "critique this design approach", "review"},
		{"implement", "implement a binary search function", "generation"},
		{"create", "create a new REST API endpoint", "generation"},
		{"write code", "write a test for the parser", "generation"},
		{"build feature", "build a caching layer for the DB queries", "generation"},
		{"generate", "generate Go structs from this JSON schema", "generation"},
		{"scaffold", "scaffold a new microservice", "generation"},
		{"explain", "explain how goroutines work", "chat"},
		{"what is", "what is a closure in Go?", "chat"},
		{"how does", "how does the GC work?", "chat"},
		{"why", "why is this approach better?", "chat"},
		{"describe", "describe the architecture of this system", "chat"},
		{"short question", "hello", "simple"},
		{"yes no", "yes", "simple"},
		{"ok", "sounds good", "simple"},
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
	roles, defaultModel := testAnthropicRoles(t)
	_, _, openaiSonnet := testTierModels(t, "openai")
	cr := NewCascadeRouter(defaultModel, roles)

	selected := cr.SelectModel("fix the bug", defaultModel, openaiSonnet)
	if selected != openaiSonnet {
		t.Errorf("user override should win, got %q", selected)
	}

	decs := cr.Decisions()
	if len(decs) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decs))
	}
	if decs[0].TaskType != "override" {
		t.Errorf("expected task type 'override', got %q", decs[0].TaskType)
	}
	if decs[0].SelectedModel != openaiSonnet {
		t.Errorf("expected selected model %q, got %q", openaiSonnet, decs[0].SelectedModel)
	}
}

func TestSelectModel_Disabled(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	haiku, _, _ := testTierModels(t, testProvider)
	cr := NewCascadeRouter(defaultModel, roles)
	cr.Enabled = false

	selected := cr.SelectModel("implement a full web framework", haiku, "")
	if selected != haiku {
		t.Errorf("disabled router should pass through current model, got %q", selected)
	}
}

func TestSelectModel_DebugRouting(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)

	selected := cr.SelectModel("fix the segfault in main.go", defaultModel, "")
	if selected != roles.Reviewer {
		t.Errorf("debug should route to reviewer, got %q", selected)
	}
}

func TestSelectModel_GenerationRouting(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)

	selected := cr.SelectModel("implement a distributed consensus algorithm", defaultModel, "")
	if selected != roles.Planner {
		t.Errorf("generation should route to planner, got %q", selected)
	}
}

func TestSelectModel_SimpleRouting(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)
	cr.FrugalMode = true

	selected := cr.SelectModel("yes", defaultModel, "")
	if selected != roles.Commit {
		t.Errorf("simple task (frugal) should route to commit, got %q", selected)
	}
}

func TestSelectModel_NoDowngradeWithoutFrugal(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)
	cr.FrugalMode = false

	selected := cr.SelectModel("ok", defaultModel, "")
	if selected != defaultModel {
		t.Errorf("without frugal, should not downgrade from default, got %q", selected)
	}
}

func TestSelectModel_FrugalDowngradesChatAndReview(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)
	cr.FrugalMode = true

	selected := cr.SelectModel("explain what a goroutine is", roles.Planner, "")
	if selected != roles.Commit {
		t.Errorf("frugal should downgrade chat to commit, got %q", selected)
	}

	selected = cr.SelectModel("review this code for issues", roles.Planner, "")
	if selected != roles.Commit {
		t.Errorf("frugal should downgrade review to commit, got %q", selected)
	}
}

func TestSelectModel_FrugalCapsGeneration(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)
	cr.FrugalMode = true

	selected := cr.SelectModel("implement a new parser", roles.Commit, "")
	if selected != roles.Coder {
		t.Errorf("frugal should cap generation at coder, got %q", selected)
	}
}

func TestTierOf(t *testing.T) {
	anthropicHaiku, anthropicSonnet, anthropicOpus := testTierModels(t, testProvider)

	tests := []struct {
		model string
		tier  routing.CostTier
	}{
		{anthropicHaiku, routing.CostTierCheap},
		{anthropicSonnet, routing.CostTierMid},
		{anthropicOpus, routing.CostTierExpensive},
		{"unknown-model-xyz", routing.CostTierMid},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if tt.model == "" {
				// FIXME: no catalog model for this provider tier in test fixture
				t.Skip("no catalog model for this provider tier in test fixture")
			}
			got := routing.CostTierOf(tt.model)
			if got != tt.tier {
				t.Errorf("CostTierOf(%q) = %v, want %v", tt.model, got, tt.tier)
			}
		})
	}
}

func TestDecisions_Tracking(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	_, _, openaiSonnet := testTierModels(t, "openai")
	cr := NewCascadeRouter(defaultModel, roles)

	if cr.DecisionCount() != 0 {
		t.Fatalf("expected 0 decisions initially, got %d", cr.DecisionCount())
	}

	cr.SelectModel("fix the bug", defaultModel, "")
	cr.SelectModel("implement a parser", defaultModel, "")
	cr.SelectModel("hello", defaultModel, openaiSonnet)

	if cr.DecisionCount() != 3 {
		t.Fatalf("expected 3 decisions, got %d", cr.DecisionCount())
	}

	decs := cr.Decisions()
	if len(decs) != 3 {
		t.Fatalf("expected 3 decisions in snapshot, got %d", len(decs))
	}
	if decs[0].TaskType != "debug" {
		t.Errorf("decision[0] task type = %q, want 'debug'", decs[0].TaskType)
	}
	if decs[1].TaskType != "generation" {
		t.Errorf("decision[1] task type = %q, want 'generation'", decs[1].TaskType)
	}
	if decs[2].TaskType != "override" {
		t.Errorf("decision[2] task type = %q, want 'override'", decs[2].TaskType)
	}
	for i, d := range decs {
		if d.Timestamp.IsZero() {
			t.Errorf("decision[%d] has zero timestamp", i)
		}
	}
}

func TestSavings(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)

	if s := cr.Savings(); s != 0 {
		t.Errorf("expected 0 savings initially, got %f", s)
	}

	openaiHaiku, _, openaiOpus := testTierModels(t, "openai")
	cr.record(openaiOpus, openaiHaiku, "simple", "test")

	savings := cr.Savings()
	if savings <= 0 {
		t.Errorf("expected positive savings for downgrade, got %f", savings)
	}
}

func TestSummary(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)

	summary := cr.Summary()
	if summary == "" {
		t.Error("expected non-empty summary even with no decisions")
	}

	cr.SelectModel("fix the bug", defaultModel, "")
	cr.SelectModel("implement a parser", defaultModel, "")

	summary = cr.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
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
	_, defaultModel := testAnthropicRoles(t)
	_, _, opus := testTierModels(t, testProvider)
	haiku, _, _ := testTierModels(t, testProvider)
	cr := NewCascadeRouter(defaultModel, routing.ModelRoles{})
	cr.FrugalMode = true

	selected := cr.SelectModel("ok", opus, "")
	if selected == "" {
		t.Error("empty roles + simple task should still return a model")
	}

	selected = cr.SelectModel("implement a compiler", haiku, "")
	if selected == "" {
		t.Error("empty roles + generation should still return a model")
	}
}

func TestSelectModel_EmptyOverrideIgnored(t *testing.T) {
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)

	selected := cr.SelectModel("fix the crash", defaultModel, "   ")
	if selected != defaultModel {
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
	roles, defaultModel := testAnthropicRoles(t)
	cr := NewCascadeRouter(defaultModel, roles)
	cr.FrugalMode = false

	selected := cr.SelectModel("implement a full distributed system", roles.Commit, "")
	if selected != roles.Planner {
		t.Errorf("should upgrade from commit to planner for generation, got %q", selected)
	}
}
