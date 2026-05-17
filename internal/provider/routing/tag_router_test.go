package routing

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewTagRouter(t *testing.T) {
	tr := NewTagRouter("claude-sonnet-4-20250514")
	if tr.DefaultModel != "claude-sonnet-4-20250514" {
		t.Fatalf("expected default model claude-sonnet-4-20250514, got %s", tr.DefaultModel)
	}
	if len(tr.Rules) == 0 {
		t.Fatal("expected built-in rules to be loaded")
	}
}

func TestAddRule(t *testing.T) {
	tr := NewTagRouter("default-model")
	initial := len(tr.Rules)

	tr.AddRule(TagRule{
		Tags:        map[string]string{"team": "backend"},
		Model:       "claude-opus-4-20250514",
		Provider:    "anthropic",
		Priority:    25,
		Description: "Backend team gets opus",
	})

	if len(tr.Rules) != initial+1 {
		t.Fatalf("expected %d rules, got %d", initial+1, len(tr.Rules))
	}
}

func TestRouteExactMatch(t *testing.T) {
	tr := NewTagRouter("default-model")

	decision := tr.Route(map[string]string{"task": "review"})
	if decision.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514 for task=review, got %s", decision.Model)
	}
	if decision.MatchedRule == nil {
		t.Fatal("expected matched rule to be set")
	}
	if decision.Provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", decision.Provider)
	}
}

func TestRouteChatTask(t *testing.T) {
	tr := NewTagRouter("default-model")

	decision := tr.Route(map[string]string{"task": "chat"})
	if decision.Model != "claude-haiku-4-20250514" {
		t.Fatalf("expected claude-haiku-4-20250514 for task=chat, got %s", decision.Model)
	}
}

func TestRouteHighComplexity(t *testing.T) {
	tr := NewTagRouter("default-model")

	decision := tr.Route(map[string]string{"complexity": "high"})
	if decision.Model != "claude-opus-4-20250514" {
		t.Fatalf("expected claude-opus-4-20250514 for complexity=high, got %s", decision.Model)
	}
}

func TestRouteCIEnvironment(t *testing.T) {
	tr := NewTagRouter("default-model")

	decision := tr.Route(map[string]string{"env": "ci"})
	if decision.Model != "claude-haiku-4-20250514" {
		t.Fatalf("expected claude-haiku-4-20250514 for env=ci, got %s", decision.Model)
	}
}

func TestRoutePriorityWins(t *testing.T) {
	tr := NewTagRouter("default-model")

	// complexity=high has priority 20, task=chat has priority 10
	// When both match, complexity=high should win
	decision := tr.Route(map[string]string{"complexity": "high", "task": "chat"})
	if decision.Model != "claude-opus-4-20250514" {
		t.Fatalf("expected opus (priority 20) to win over haiku (priority 10), got %s", decision.Model)
	}
}

func TestRouteNoMatch(t *testing.T) {
	tr := NewTagRouter("fallback-model")

	decision := tr.Route(map[string]string{"team": "unknown"})
	if decision.Model != "fallback-model" {
		t.Fatalf("expected fallback-model, got %s", decision.Model)
	}
	if decision.MatchedRule != nil {
		t.Fatal("expected no matched rule")
	}
	if !strings.Contains(decision.Reason, "no rules matched") {
		t.Fatalf("expected reason to mention no match, got %s", decision.Reason)
	}
}

func TestRouteEmptyTags(t *testing.T) {
	tr := NewTagRouter("fallback-model")

	decision := tr.Route(map[string]string{})
	if decision.Model != "fallback-model" {
		t.Fatalf("expected fallback-model for empty tags, got %s", decision.Model)
	}
	if !strings.Contains(decision.Reason, "no tags provided") {
		t.Fatalf("expected reason to mention no tags, got %s", decision.Reason)
	}
}

func TestRouteNilTags(t *testing.T) {
	tr := NewTagRouter("fallback-model")

	decision := tr.Route(nil)
	if decision.Model != "fallback-model" {
		t.Fatalf("expected fallback-model for nil tags, got %s", decision.Model)
	}
}

func TestRouteByContext(t *testing.T) {
	tr := NewTagRouter("default-model")

	tests := []struct {
		task       string
		language   string
		complexity string
		wantModel  string
	}{
		{"review", "", "", "claude-sonnet-4-20250514"},
		{"chat", "", "", "claude-haiku-4-20250514"},
		{"", "", "high", "claude-opus-4-20250514"},
		{"", "python", "", "claude-sonnet-4-20250514"},
		{"", "", "", "default-model"},
	}

	for _, tt := range tests {
		decision := tr.RouteByContext(tt.task, tt.language, tt.complexity)
		if decision.Model != tt.wantModel {
			t.Errorf("RouteByContext(%q, %q, %q) = %s, want %s",
				tt.task, tt.language, tt.complexity, decision.Model, tt.wantModel)
		}
	}
}

func TestMatchScore(t *testing.T) {
	tests := []struct {
		name      string
		rule      TagRule
		tags      map[string]string
		wantScore int
	}{
		{
			name:      "single tag match",
			rule:      TagRule{Tags: map[string]string{"task": "review"}},
			tags:      map[string]string{"task": "review"},
			wantScore: 1,
		},
		{
			name:      "multi tag match",
			rule:      TagRule{Tags: map[string]string{"task": "review", "env": "ci"}},
			tags:      map[string]string{"task": "review", "env": "ci", "team": "backend"},
			wantScore: 2,
		},
		{
			name:      "partial match returns 0",
			rule:      TagRule{Tags: map[string]string{"task": "review", "env": "ci"}},
			tags:      map[string]string{"task": "review"},
			wantScore: 0,
		},
		{
			name:      "no match",
			rule:      TagRule{Tags: map[string]string{"task": "review"}},
			tags:      map[string]string{"task": "chat"},
			wantScore: 0,
		},
		{
			name:      "empty rule tags",
			rule:      TagRule{Tags: map[string]string{}},
			tags:      map[string]string{"task": "review"},
			wantScore: 0,
		},
		{
			name:      "empty request tags",
			rule:      TagRule{Tags: map[string]string{"task": "review"}},
			tags:      map[string]string{},
			wantScore: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := MatchScore(tt.rule, tt.tags)
			if score != tt.wantScore {
				t.Errorf("MatchScore() = %d, want %d", score, tt.wantScore)
			}
		})
	}
}

func TestMatchScoreBreaksTies(t *testing.T) {
	tr := NewTagRouter("default-model")

	// Add a multi-tag rule with priority 15 (same as the env=ci built-in rule).
	// Both match when tags include task=review and env=ci, but the multi-tag
	// rule has a higher match score (2 vs 1) and should win the tie-break.
	tr.AddRule(TagRule{
		Tags:        map[string]string{"task": "review", "env": "ci"},
		Model:       "special-ci-review-model",
		Provider:    "anthropic",
		Priority:    15,
		Description: "CI review: specific model for CI reviews",
	})

	decision := tr.Route(map[string]string{"task": "review", "env": "ci"})
	if decision.Model != "special-ci-review-model" {
		t.Fatalf("expected multi-tag rule to win tie-break, got %s", decision.Model)
	}
}

func TestFormatRules(t *testing.T) {
	tr := NewTagRouter("default-model")

	output := tr.FormatRules()
	if !strings.Contains(output, "Tag Router Rules") {
		t.Fatal("expected header in formatted output")
	}
	if !strings.Contains(output, "default-model") {
		t.Fatal("expected default model in formatted output")
	}
	if !strings.Contains(output, "priority=") {
		t.Fatal("expected priority info in formatted output")
	}
	if !strings.Contains(output, "Code review") {
		t.Fatal("expected rule description in formatted output")
	}
}

func TestFormatRulesEmpty(t *testing.T) {
	tr := &TagRouter{
		DefaultModel: "model",
		Rules:        nil,
	}

	output := tr.FormatRules()
	if !strings.Contains(output, "No routing rules configured") {
		t.Fatal("expected empty message for no rules")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tag_router.json")

	// Create and save
	tr := NewTagRouter("default-model")
	tr.AddRule(TagRule{
		Tags:        map[string]string{"team": "platform"},
		Model:       "custom-model",
		Provider:    "openai",
		Priority:    50,
		Description: "Platform team custom rule",
	})

	if err := tr.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file not found: %v", err)
	}

	// Load into new router
	tr2 := &TagRouter{}
	if err := tr2.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if tr2.DefaultModel != "default-model" {
		t.Fatalf("loaded default model = %s, want default-model", tr2.DefaultModel)
	}

	// Check that custom rule is present
	decision := tr2.Route(map[string]string{"team": "platform"})
	if decision.Model != "custom-model" {
		t.Fatalf("loaded router did not preserve custom rule, got model %s", decision.Model)
	}
}

func TestLoadNonexistent(t *testing.T) {
	tr := &TagRouter{}
	err := tr.Load("/nonexistent/path/file.json")
	if err == nil {
		t.Fatal("expected error loading nonexistent file")
	}
}

func TestSaveInvalidPath(t *testing.T) {
	tr := NewTagRouter("model")
	err := tr.Save("/nonexistent/dir/file.json")
	if err == nil {
		t.Fatal("expected error saving to invalid path")
	}
}

func TestTagRouterConcurrentAccess(t *testing.T) {
	tr := NewTagRouter("default-model")
	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Route(map[string]string{"task": "review"})
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tr.AddRule(TagRule{
				Tags:     map[string]string{"team": "concurrent"},
				Model:    "model",
				Provider: "p",
				Priority: n,
			})
		}(i)
	}

	// Concurrent format
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.FormatRules()
		}()
	}

	wg.Wait()
}

func TestCustomRuleHigherPriority(t *testing.T) {
	tr := NewTagRouter("default-model")

	// Add a high-priority custom rule that overrides the built-in review rule
	tr.AddRule(TagRule{
		Tags:        map[string]string{"task": "review"},
		Model:       "custom-review-model",
		Provider:    "openai",
		Priority:    100,
		Description: "Override: custom review model",
	})

	decision := tr.Route(map[string]string{"task": "review"})
	if decision.Model != "custom-review-model" {
		t.Fatalf("expected custom rule to override built-in, got %s", decision.Model)
	}
}

func TestRouteByContextEmpty(t *testing.T) {
	tr := NewTagRouter("default-model")

	decision := tr.RouteByContext("", "", "")
	if decision.Model != "default-model" {
		t.Fatalf("expected default model for empty context, got %s", decision.Model)
	}
}
