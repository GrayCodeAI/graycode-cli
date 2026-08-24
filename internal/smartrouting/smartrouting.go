// Package smartrouting provides a deterministic, pure classifier that decides
// whether a user turn should go to a cheap "simple" model or a strong model.
// It is the Go port of OpenClaude's smart model routing: trivial turns ("ok",
// "rename this", "what does this do?") route to a cheaper model while the
// strong model handles anything non-trivial. When in doubt it routes to the
// strong model, so the failure mode is "no savings on a cheap turn," never a
// silently degraded answer on a turn you cared about. The classifier is a pure
// function — the caller supplies config and input.
package smartrouting

import "strings"

// Config selects the simple and strong models and the size thresholds that
// qualify a turn as "simple". Enabled is opt-in.
type Config struct {
	Enabled        bool
	SimpleModel    string
	StrongModel    string
	SimpleMax      int // max characters to qualify as simple (default 160)
	SimpleMaxWords int // max whitespace-separated words (default 28)
}

// Complexity is the classifier's verdict.
type Complexity string

const (
	Simple Complexity = "simple"
	Strong Complexity = "strong"
)

// Input describes the user turn to classify.
type Input struct {
	UserText   string
	HasNonText bool // image/document or other non-text blocks
	TurnNumber int  // 1-indexed turn in the session
}

// Decision is the classifier's output.
type Decision struct {
	Model      string
	Complexity Complexity
	Reason     string
}

const (
	defaultSimpleMax      = 160
	defaultSimpleMaxWords = 28
)

// strongKeywords strongly suggest reasoning/planning/design work.
var strongKeywords = []string{
	"plan", "design", "architect", "architecture", "refactor", "debug",
	"investigate", "analyze", "analyse", "implement", "optimize", "optimise",
	"review", "audit", "diagnose", "root cause", "root-cause", "why does",
	"why is", "how should", "why did", "propose", "trace", "reproduce",
}

// Route decides which model to use for the turn.
func Route(input Input, cfg Config) Decision {
	if !cfg.Enabled {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "smart-routing disabled"}
	}
	if cfg.SimpleModel == "" || cfg.StrongModel == "" {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "simpleModel or strongModel missing"}
	}
	if cfg.SimpleModel == cfg.StrongModel {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "simpleModel equals strongModel"}
	}

	text := strings.TrimSpace(input.UserText)

	if input.HasNonText {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "contains non-text content"}
	}
	if text == "" {
		return Decision{Model: cfg.SimpleModel, Complexity: Simple, Reason: "empty user text"}
	}
	if input.TurnNumber == 1 {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "first turn of session"}
	}

	maxChars := cfg.SimpleMax
	if maxChars <= 0 {
		maxChars = defaultSimpleMax
	}
	maxWords := cfg.SimpleMaxWords
	if maxWords <= 0 {
		maxWords = defaultSimpleMaxWords
	}

	if hasCode(text) {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "contains code block or inline code"}
	}
	if hasStrongKeyword(text) {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "contains reasoning/planning keyword"}
	}
	if hasMultiParagraph(text) {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "multi-paragraph input"}
	}
	if len(text) > maxChars {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "input exceeds char threshold"}
	}
	if countWords(text) > maxWords {
		return Decision{Model: cfg.StrongModel, Complexity: Strong, Reason: "input exceeds word threshold"}
	}
	return Decision{
		Model:      cfg.SimpleModel,
		Complexity: Simple,
		Reason:     "short input",
	}
}

// hasCode detects a fenced code block or an inline backtick span.
func hasCode(text string) bool {
	if strings.Contains(text, "```") {
		return true
	}
	return strings.Contains(text, "`")
}

// hasStrongKeyword matches reasoning/planning keywords with word boundaries,
// case-insensitive.
func hasStrongKeyword(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range strongKeywords {
		if containsWord(lower, kw) {
			return true
		}
	}
	return false
}

func containsWord(haystack, needle string) bool {
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return false
		}
		pos := start + idx
		beforeOK := pos == 0 || !isWordByte(haystack[pos-1])
		end := pos + len(needle)
		afterOK := end >= len(haystack) || !isWordByte(haystack[end])
		if beforeOK && afterOK {
			return true
		}
		start = pos + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func hasMultiParagraph(text string) bool {
	return strings.Contains(text, "\n\n") || strings.Contains(text, "\n \n")
}

func countWords(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return len(strings.Fields(trimmed))
}
