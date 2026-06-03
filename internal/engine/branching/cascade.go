package branching

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/cost"
	"github.com/GrayCodeAI/hawk/internal/provider/routing"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

// CascadeRouter selects the optimal model for each request based on task complexity.
// It uses pre-request classification (before we have a response) to route:
//   - simple/chat tasks -> cheap model (haiku)
//   - debug/review/refactor -> mid model (sonnet)
//   - generation -> expensive model (opus)
//
// The router also tracks routing decisions for analytics.
type CascadeRouter struct {
	Enabled      bool
	FrugalMode   bool   // more aggressive downgrading
	DefaultModel string // fallback when classification is inconclusive
	Roles        routing.ModelRoles

	mu        sync.Mutex
	decisions []RoutingDecision
}

// RoutingDecision records a single model selection event.
type RoutingDecision struct {
	OriginalModel string    `json:"original_model"`
	SelectedModel string    `json:"selected_model"`
	TaskType      string    `json:"task_type"`
	Reason        string    `json:"reason"`
	Timestamp     time.Time `json:"timestamp"`
}

// ModelTier represents the cost tier of a model.
type ModelTier int

const (
	TierCheap     ModelTier = iota // haiku-class
	TierMid                        // sonnet-class
	TierExpensive                  // opus-class
)

// NewCascadeRouter creates a router with sensible defaults.
// The defaultModel is used as the fallback when classification yields no
// strong signal and no role-specific model is configured.
func NewCascadeRouter(defaultModel string, roles routing.ModelRoles) *CascadeRouter {
	return &CascadeRouter{
		Enabled:      true,
		DefaultModel: defaultModel,
		Roles:        roles,
	}
}

// SelectModel picks the best model for a given prompt. If userOverride is
// non-empty the user's explicit choice always wins (override is never
// downgraded). The returned string is the model name to use for the API call.
func (cr *CascadeRouter) SelectModel(prompt string, currentModel string, userOverride string) string {
	cr.mu.Lock()
	enabled := cr.Enabled
	frugal := cr.FrugalMode
	cr.mu.Unlock()

	if !enabled {
		return cr.pick(currentModel)
	}

	// User-explicit override always wins.
	if strings.TrimSpace(userOverride) != "" {
		cr.record(currentModel, userOverride, "override", "user explicitly selected model")
		return userOverride
	}

	taskType := classifyPrompt(prompt)
	selected := cr.modelForTask(taskType, frugal)
	if strings.TrimSpace(selected) == "" {
		selected = cr.pick(currentModel)
	}

	// When frugal mode is off, never downgrade from what was already set --
	// only upgrade or keep the same tier.
	if !frugal && strings.TrimSpace(selected) != "" && strings.TrimSpace(currentModel) != "" &&
		routing.CostTierOf(selected) < routing.CostTierOf(currentModel) {
		selected = currentModel
	}
	if strings.TrimSpace(selected) == "" {
		selected = cr.pick(currentModel)
	}

	reason := fmt.Sprintf("classified as %q", taskType)
	if frugal {
		reason += " (frugal)"
	}
	cr.record(currentModel, selected, taskType, reason)
	return selected
}

// Decisions returns a snapshot of all routing decisions made so far.
func (cr *CascadeRouter) Decisions() []RoutingDecision {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	out := make([]RoutingDecision, len(cr.decisions))
	copy(out, cr.decisions)
	return out
}

// DecisionCount returns how many routing decisions have been recorded.
func (cr *CascadeRouter) DecisionCount() int {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return len(cr.decisions)
}

// Savings estimates the USD saved by routing decisions compared to always
// using the most expensive model. This is a rough heuristic: it sums the
// per-million-token price difference for each decision where the selected
// model is cheaper than the original.
func (cr *CascadeRouter) Savings() float64 {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	var saved float64
	for _, d := range cr.decisions {
		origIn, _ := cost.ModelPricing(d.OriginalModel)
		selIn, _ := cost.ModelPricing(d.SelectedModel)
		if selIn < origIn {
			// Rough estimate: assume 4000 input tokens per request.
			saved += (origIn - selIn) * 4000 / 1_000_000
		}
	}
	return saved
}

// Summary returns a human-readable summary of routing activity.
func (cr *CascadeRouter) Summary() string {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if len(cr.decisions) == 0 {
		return "Cascade router: no decisions recorded."
	}

	counts := map[string]int{}
	downgrades := 0
	upgrades := 0
	unchanged := 0
	for _, d := range cr.decisions {
		counts[d.TaskType]++
		origTier := routing.CostTierOf(d.OriginalModel)
		selTier := routing.CostTierOf(d.SelectedModel)
		switch {
		case selTier < origTier:
			downgrades++
		case selTier > origTier:
			upgrades++
		default:
			unchanged++
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Cascade router: %d decisions", len(cr.decisions)))
	b.WriteString(fmt.Sprintf(" (%d downgrades, %d upgrades, %d unchanged)\n", downgrades, upgrades, unchanged))
	for task, n := range counts {
		b.WriteString(fmt.Sprintf("  %-12s %d\n", task, n))
	}
	return b.String()
}

// classifyPrompt determines the task type from the prompt alone (pre-response).
// It mirrors the keyword heuristics in analytics.ClassifyTask but operates
// without a response, so response-length and code-block checks are skipped.
func classifyPrompt(prompt string) string {
	lower := strings.ToLower(prompt)

	// Debug: error-related prompts.
	if promptContainsAny(lower, "fix", "bug", "error", "debug", "broken", "failing", "crash", "panic", "stack trace") {
		return "debug"
	}

	// Refactor: restructuring-related prompts.
	if promptContainsAny(lower, "refactor", "rename", "reorganize", "simplify", "clean up", "restructure", "extract") {
		return "refactor"
	}

	// Review: code review prompts.
	if promptContainsAny(lower, "review", "check this", "look over", "feedback", "critique", "audit") {
		return "review"
	}

	// Generation: code creation prompts.
	if promptContainsAny(lower, "implement", "create", "write", "build", "generate", "scaffold", "add a new") {
		return "generation"
	}

	// Chat / explanation: conversational prompts.
	if promptContainsAny(lower, "explain", "what is", "how does", "why", "tell me", "describe") {
		return "chat"
	}

	// Simple: very short prompts with no strong signal.
	words := strings.Fields(prompt)
	if len(words) <= 8 {
		return "simple"
	}

	return "chat"
}

// modelForTask maps a task type to the appropriate model using configured roles
// and eyrie catalog tier defaults.
func (cr *CascadeRouter) modelForTask(taskType string, frugal bool) string {
	tier := routing.SuggestTierForTask(taskType)

	switch tier {
	case eycatalog.TierHaiku:
		if m := cr.Roles.Commit; m != "" {
			return m
		}
		return cr.defaultFor(TierCheap)
	case eycatalog.TierSonnet:
		if frugal {
			if taskType == "chat" || taskType == "review" {
				if m := cr.Roles.Commit; m != "" {
					return m
				}
				return cr.defaultFor(TierCheap)
			}
		}
		if m := cr.Roles.Reviewer; m != "" {
			return m
		}
		return cr.defaultFor(TierMid)
	case eycatalog.TierOpus:
		if frugal {
			if m := cr.Roles.Coder; m != "" {
				return m
			}
			return cr.defaultFor(TierMid)
		}
		if m := cr.Roles.Planner; m != "" {
			return m
		}
		return cr.defaultFor(TierExpensive)
	default:
		return cr.pick("")
	}
}

// defaultFor returns the best model for a given cost tier via eyrie catalog tier defaults.
func (cr *CascadeRouter) defaultFor(tier ModelTier) string {
	provider := ""
	if info, ok := routing.Find(cr.DefaultModel); ok {
		provider = info.Provider
	}
	switch tier {
	case TierCheap:
		return routing.CheapestForProvider(provider, cr.pick(""))
	case TierExpensive:
		return routing.MostExpensiveForProvider(provider, cr.pick(""))
	default:
		return routing.PreferredModelForTier(provider, eycatalog.TierSonnet, cr.pick(""))
	}
}

// pick returns m if non-empty, otherwise the router's DefaultModel.
func (cr *CascadeRouter) pick(m string) string {
	if strings.TrimSpace(m) != "" {
		return m
	}
	if strings.TrimSpace(cr.DefaultModel) != "" {
		return cr.DefaultModel
	}
	return ""
}

// record appends a routing decision to the history.
func (cr *CascadeRouter) record(original, selected, taskType, reason string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.decisions = append(cr.decisions, RoutingDecision{
		OriginalModel: original,
		SelectedModel: selected,
		TaskType:      taskType,
		Reason:        reason,
		Timestamp:     time.Now(),
	})
}

// promptContainsAny checks whether s contains any of the given substrings.
// This is the engine-local equivalent of analytics.containsAny (which is
// unexported).
func promptContainsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
