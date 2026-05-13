package engine

import (
	"fmt"
	"strings"
	"sync"
)

// Intent category constants.
const (
	IntentCodeWrite  = "code_write"
	IntentCodeFix    = "code_fix"
	IntentCodeReview = "code_review"
	IntentExplain    = "explain"
	IntentRefactor   = "refactor"
	IntentTest       = "test"
	IntentSearch     = "search"
	IntentConfig     = "config"
	IntentGit        = "git"
	IntentQuestion   = "question"
)

// Intent represents a classified user intent with confidence and metadata.
type Intent struct {
	Category            string
	Confidence          float64
	SubCategory         string
	Keywords            []string
	SuggestedTools      []string
	EstimatedComplexity string
}

// IntentRule defines a pattern-matching rule for intent classification.
type IntentRule struct {
	Category    string
	SubCategory string
	Patterns    []string
	Weight      float64
	Tools       []string
}

// ClassifiedInput records a classification for history tracking.
type ClassifiedInput struct {
	Input  string
	Intent *Intent
}

// IntentClassifier categorizes user messages to route them to appropriate
// handling strategies before involving the LLM.
type IntentClassifier struct {
	Rules   []IntentRule
	History []ClassifiedInput
	mu      sync.RWMutex
}

// NewIntentClassifier creates an IntentClassifier with default keyword rules
// for each intent category.
func NewIntentClassifier() *IntentClassifier {
	ic := &IntentClassifier{
		Rules: []IntentRule{
			{
				Category:    IntentCodeWrite,
				SubCategory: "implement",
				Patterns:    []string{"implement", "create", "add feature", "write", "build", "generate", "scaffold", "new file", "add function", "add method"},
				Weight:      1.0,
				Tools:       []string{"Read", "Edit", "Write", "Bash"},
			},
			{
				Category:    IntentCodeFix,
				SubCategory: "debug",
				Patterns:    []string{"fix", "debug", "resolve", "repair", "bug", "error", "broken", "crash", "failing", "null pointer", "panic", "not working"},
				Weight:      1.0,
				Tools:       []string{"Read", "Grep", "Edit", "Bash"},
			},
			{
				Category:    IntentCodeReview,
				SubCategory: "review",
				Patterns:    []string{"review", "check", "audit", "inspect", "look at", "code quality", "lint", "analyze code"},
				Weight:      1.0,
				Tools:       []string{"Read", "Grep", "Glob"},
			},
			{
				Category:    IntentExplain,
				SubCategory: "explain",
				Patterns:    []string{"explain", "what does", "how does", "why", "what is", "describe", "walk me through", "help me understand"},
				Weight:      0.9,
				Tools:       []string{"Read", "Glob"},
			},
			{
				Category:    IntentRefactor,
				SubCategory: "restructure",
				Patterns:    []string{"refactor", "restructure", "improve", "optimize", "clean up", "simplify", "rename", "extract", "reorganize"},
				Weight:      1.0,
				Tools:       []string{"Read", "Edit", "Grep", "Bash"},
			},
			{
				Category:    IntentTest,
				SubCategory: "test",
				Patterns:    []string{"test", "coverage", "verify", "unit test", "integration test", "benchmark", "add tests", "write tests"},
				Weight:      1.0,
				Tools:       []string{"Bash", "Read", "Write"},
			},
			{
				Category:    IntentSearch,
				SubCategory: "find",
				Patterns:    []string{"find", "where is", "locate", "grep", "search", "look for", "which file", "show me"},
				Weight:      0.9,
				Tools:       []string{"Grep", "Glob", "LS"},
			},
			{
				Category:    IntentConfig,
				SubCategory: "setup",
				Patterns:    []string{"configure", "setup", "install", "config", "settings", "environment", "dependency", "dependencies", "init"},
				Weight:      0.8,
				Tools:       []string{"Read", "Edit", "Bash"},
			},
			{
				Category:    IntentGit,
				SubCategory: "version_control",
				Patterns:    []string{"commit", "push", "branch", "merge", "pull request", "rebase", "cherry-pick", "stash", "diff", "git", "new branch"},
				Weight:      1.0,
				Tools:       []string{"Bash"},
			},
			{
				Category:    IntentQuestion,
				SubCategory: "general",
				Patterns:    []string{"what", "how", "can you", "is it possible", "do you", "should i", "would"},
				Weight:      0.5,
				Tools:       nil,
			},
		},
		History: make([]ClassifiedInput, 0, 64),
	}
	return ic
}

// Classify analyzes the input string and returns the best matching Intent.
func (ic *IntentClassifier) Classify(input string) *Intent {
	lower := strings.ToLower(input)

	type scored struct {
		rule     IntentRule
		score    float64
		matched  []string
	}

	var best scored
	for _, rule := range ic.Rules {
		var matches []string
		var specificityBonus float64
		for _, pattern := range rule.Patterns {
			if strings.Contains(lower, pattern) {
				matches = append(matches, pattern)
				// Multi-word patterns are more specific and score higher
				if strings.Contains(pattern, " ") {
					specificityBonus += 0.15
				}
			}
		}
		if len(matches) == 0 {
			continue
		}
		// Score: number of matches * weight, normalized by pattern count
		score := (float64(len(matches)) / float64(len(rule.Patterns))) * rule.Weight
		// Bonus for multiple keyword hits
		if len(matches) > 1 {
			score += 0.1 * float64(len(matches)-1)
		}
		// Add specificity bonus for multi-word pattern matches
		score += specificityBonus
		if score > best.score {
			best = scored{rule: rule, score: score, matched: matches}
		}
	}

	if best.score == 0 {
		return &Intent{
			Category:            IntentQuestion,
			Confidence:          0.3,
			SubCategory:         "unknown",
			Keywords:            nil,
			SuggestedTools:      nil,
			EstimatedComplexity: ic.EstimateComplexity(input),
		}
	}

	// Cap confidence at 1.0
	confidence := best.score
	if confidence > 1.0 {
		confidence = 1.0
	}

	intent := &Intent{
		Category:            best.rule.Category,
		Confidence:          confidence,
		SubCategory:         best.rule.SubCategory,
		Keywords:            best.matched,
		SuggestedTools:      ic.SuggestTools(&Intent{Category: best.rule.Category}),
		EstimatedComplexity: ic.EstimateComplexity(input),
	}
	return intent
}

// ClassifyForRouting performs quick classification returning recommended model
// tier and tool set string.
func (ic *IntentClassifier) ClassifyForRouting(input string) (model, tools string) {
	intent := ic.Classify(input)

	// Determine model tier based on complexity and category
	switch intent.EstimatedComplexity {
	case "trivial":
		model = "fast"
	case "simple":
		model = "fast"
	case "moderate":
		model = "standard"
	case "complex":
		model = "advanced"
	default:
		model = "standard"
	}

	// Override for categories that benefit from stronger models
	switch intent.Category {
	case IntentCodeWrite, IntentRefactor:
		if model == "fast" {
			model = "standard"
		}
	case IntentCodeReview:
		if model == "fast" {
			model = "standard"
		}
	}

	// Format tool set
	if len(intent.SuggestedTools) > 0 {
		tools = strings.Join(intent.SuggestedTools, ",")
	} else {
		tools = "Read"
	}

	return model, tools
}

// SuggestTools returns the recommended tool set for the given intent category.
func (ic *IntentClassifier) SuggestTools(intent *Intent) []string {
	switch intent.Category {
	case IntentCodeWrite:
		return []string{"Read", "Edit", "Write", "Bash"}
	case IntentCodeFix:
		return []string{"Read", "Grep", "Edit", "Bash"}
	case IntentCodeReview:
		return []string{"Read", "Grep", "Glob"}
	case IntentExplain:
		return []string{"Read", "Glob"}
	case IntentRefactor:
		return []string{"Read", "Edit", "Grep", "Bash"}
	case IntentTest:
		return []string{"Bash", "Read", "Write"}
	case IntentSearch:
		return []string{"Grep", "Glob", "LS"}
	case IntentConfig:
		return []string{"Read", "Edit", "Bash"}
	case IntentGit:
		return []string{"Bash"}
	case IntentQuestion:
		return nil
	default:
		return []string{"Read"}
	}
}

// EstimateComplexity estimates how complex a request is based on word count,
// keyword density, and file mentions.
func (ic *IntentClassifier) EstimateComplexity(input string) string {
	words := strings.Fields(input)
	wordCount := len(words)

	// Count file-like mentions (paths, extensions)
	fileMentions := 0
	for _, w := range words {
		if strings.Contains(w, "/") || strings.Contains(w, ".go") ||
			strings.Contains(w, ".ts") || strings.Contains(w, ".py") ||
			strings.Contains(w, ".js") || strings.Contains(w, ".rs") {
			fileMentions++
		}
	}

	// Count action keywords
	actionKeywords := 0
	lower := strings.ToLower(input)
	actions := []string{"and", "then", "also", "additionally", "plus", "as well", "after that"}
	for _, a := range actions {
		if strings.Contains(lower, a) {
			actionKeywords++
		}
	}

	// Scoring heuristic based on combined signals
	switch {
	case wordCount <= 5 && fileMentions == 0 && actionKeywords == 0:
		return "trivial"
	case wordCount <= 15 && fileMentions <= 1 && actionKeywords == 0:
		return "simple"
	case actionKeywords >= 3:
		return "complex"
	case wordCount > 40 && actionKeywords >= 2:
		return "complex"
	case fileMentions > 3 && actionKeywords >= 2:
		return "complex"
	case wordCount > 60:
		return "complex"
	default:
		return "moderate"
	}
}

// FormatIntent returns a human-readable summary of the classified intent.
func FormatIntent(intent *Intent) string {
	keywords := ""
	if len(intent.Keywords) > 0 {
		quoted := make([]string, len(intent.Keywords))
		for i, kw := range intent.Keywords {
			quoted[i] = fmt.Sprintf("%q", kw)
		}
		keywords = strings.Join(quoted, ", ")
	}

	tools := "[]"
	if len(intent.SuggestedTools) > 0 {
		tools = "[" + strings.Join(intent.SuggestedTools, ", ") + "]"
	}

	return fmt.Sprintf("Intent: %s (confidence: %.2f)\nSub: %s\nKeywords: %s\nTools: %s\nComplexity: %s",
		intent.Category,
		intent.Confidence,
		intent.SubCategory,
		keywords,
		tools,
		intent.EstimatedComplexity,
	)
}

// RecordClassification stores a classification result in history for pattern analysis.
func (ic *IntentClassifier) RecordClassification(input string, intent *Intent) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.History = append(ic.History, ClassifiedInput{
		Input:  input,
		Intent: intent,
	})
}

// GetPatterns returns a map of intent categories to their occurrence count
// from classification history.
func (ic *IntentClassifier) GetPatterns() map[string]int {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	counts := make(map[string]int)
	for _, ci := range ic.History {
		if ci.Intent != nil {
			counts[ci.Intent.Category]++
		}
	}
	return counts
}
