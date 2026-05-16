package routing

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// TagRule defines a routing rule that maps metadata tags to a model/provider.
type TagRule struct {
	Tags        map[string]string `json:"tags"`
	Model       string            `json:"model"`
	Provider    string            `json:"provider"`
	Priority    int               `json:"priority"`
	Description string            `json:"description"`
}

// RoutingDecision holds the result of tag-based routing.
type RoutingDecision struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider"`
	MatchedRule *TagRule `json:"matched_rule,omitempty"`
	Reason      string   `json:"reason"`
}

// TagRouter routes LLM requests to specific models based on metadata tags.
type TagRouter struct {
	Rules        []TagRule `json:"rules"`
	DefaultModel string    `json:"default_model"`
	mu           sync.RWMutex
}

// NewTagRouter creates a new TagRouter with a default model and built-in rules.
func NewTagRouter(defaultModel string) *TagRouter {
	tr := &TagRouter{
		DefaultModel: defaultModel,
		Rules:        builtinRules(),
	}
	return tr
}

// builtinRules returns the default set of tag-based routing rules.
func builtinRules() []TagRule {
	return []TagRule{
		{
			Tags:        map[string]string{"task": "review"},
			Model:       "claude-sonnet-4-20250514",
			Provider:    "anthropic",
			Priority:    10,
			Description: "Code review: precise model for thorough analysis",
		},
		{
			Tags:        map[string]string{"task": "chat"},
			Model:       "claude-haiku-4-20250514",
			Provider:    "anthropic",
			Priority:    10,
			Description: "Chat: fast and cheap model for conversational tasks",
		},
		{
			Tags:        map[string]string{"complexity": "high"},
			Model:       "claude-opus-4-20250514",
			Provider:    "anthropic",
			Priority:    20,
			Description: "High complexity: most capable model for hard problems",
		},
		{
			Tags:        map[string]string{"language": "python"},
			Model:       "claude-sonnet-4-20250514",
			Provider:    "anthropic",
			Priority:    5,
			Description: "Python tasks: strong Python model",
		},
		{
			Tags:        map[string]string{"env": "ci"},
			Model:       "claude-haiku-4-20250514",
			Provider:    "anthropic",
			Priority:    15,
			Description: "CI environment: fastest and cheapest model",
		},
	}
}

// AddRule adds a routing rule to the router.
func (tr *TagRouter) AddRule(rule TagRule) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.Rules = append(tr.Rules, rule)
}

// Route finds the best model for the given tags by matching against rules.
// All tags in a rule must match the request tags for the rule to apply.
// Among matching rules, the highest priority wins. Ties are broken by match score.
func (tr *TagRouter) Route(tags map[string]string) *RoutingDecision {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if len(tags) == 0 {
		return &RoutingDecision{
			Model:  tr.DefaultModel,
			Reason: "no tags provided, using default model",
		}
	}

	type candidate struct {
		rule  TagRule
		score int
	}

	var candidates []candidate
	for _, rule := range tr.Rules {
		score := MatchScore(rule, tags)
		if score > 0 {
			candidates = append(candidates, candidate{rule: rule, score: score})
		}
	}

	if len(candidates) == 0 {
		return &RoutingDecision{
			Model:  tr.DefaultModel,
			Reason: "no rules matched, using default model",
		}
	}

	// Sort: highest priority first, then highest match score
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].rule.Priority != candidates[j].rule.Priority {
			return candidates[i].rule.Priority > candidates[j].rule.Priority
		}
		return candidates[i].score > candidates[j].score
	})

	winner := candidates[0]
	return &RoutingDecision{
		Model:       winner.rule.Model,
		Provider:    winner.rule.Provider,
		MatchedRule: &winner.rule,
		Reason:      fmt.Sprintf("matched rule: %s (priority=%d, score=%d)", winner.rule.Description, winner.rule.Priority, winner.score),
	}
}

// RouteByContext auto-generates tags from context parameters and routes accordingly.
func (tr *TagRouter) RouteByContext(task, language, complexity string) *RoutingDecision {
	tags := make(map[string]string)
	if task != "" {
		tags["task"] = task
	}
	if language != "" {
		tags["language"] = language
	}
	if complexity != "" {
		tags["complexity"] = complexity
	}
	return tr.Route(tags)
}

// MatchScore returns the number of matching tags between a rule and a tag set.
// Returns 0 if any tag in the rule does not match (all rule tags must be present).
func MatchScore(rule TagRule, tags map[string]string) int {
	if len(rule.Tags) == 0 {
		return 0
	}
	matched := 0
	for k, v := range rule.Tags {
		tagVal, ok := tags[k]
		if !ok || tagVal != v {
			return 0
		}
		matched++
	}
	return matched
}

// FormatRules returns a human-readable summary of all routing rules.
func (tr *TagRouter) FormatRules() string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if len(tr.Rules) == 0 {
		return "No routing rules configured."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tag Router Rules (default: %s)\n", tr.DefaultModel))
	b.WriteString(strings.Repeat("-", 60) + "\n")

	// Sort by priority descending for display
	sorted := make([]TagRule, len(tr.Rules))
	copy(sorted, tr.Rules)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	for i, rule := range sorted {
		tags := formatTags(rule.Tags)
		b.WriteString(fmt.Sprintf("%d. [priority=%d] %s\n", i+1, rule.Priority, rule.Description))
		b.WriteString(fmt.Sprintf("   Tags: %s -> Model: %s (Provider: %s)\n", tags, rule.Model, rule.Provider))
	}
	return b.String()
}

// formatTags returns a readable representation of tag key-value pairs.
func formatTags(tags map[string]string) string {
	if len(tags) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(tags))
	for k, v := range tags {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ", ") + "}"
}

// Save persists the router's rules to a JSON file at the given path.
func (tr *TagRouter) Save(path string) error {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	data, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tag router: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write tag router config: %w", err)
	}
	return nil
}

// Load reads the router's rules from a JSON file at the given path.
func (tr *TagRouter) Load(path string) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read tag router config: %w", err)
	}

	var loaded TagRouter
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("unmarshal tag router config: %w", err)
	}

	tr.Rules = loaded.Rules
	tr.DefaultModel = loaded.DefaultModel
	return nil
}
