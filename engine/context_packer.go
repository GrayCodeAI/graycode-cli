package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// PackingStrategy determines how the context packer selects messages to keep.
type PackingStrategy string

const (
	// StrategyRecent keeps the most recent messages (simple truncation).
	StrategyRecent PackingStrategy = "recent"

	// StrategyRelevance scores messages by relevance to the current task.
	StrategyRelevance PackingStrategy = "relevance"

	// StrategyHybrid combines recency and relevance scoring (default).
	StrategyHybrid PackingStrategy = "hybrid"

	// StrategyCompression summarizes old messages, keeps recent verbatim.
	StrategyCompression PackingStrategy = "compression"
)

// ContextPacker optimally selects which messages to keep when approaching
// context window limits, maximizing information density per token spent.
type ContextPacker struct {
	MaxTokens          int             // model's context window
	ReservedForOutput  int             // tokens reserved for response
	SystemPromptTokens int             // tokens consumed by system prompt
	Strategy           PackingStrategy // packing strategy to use
}

// ScoredMessage represents a message with scoring metadata for packing decisions.
type ScoredMessage struct {
	Index    int
	Role     string
	Content  string
	Tokens   int
	Score    float64
	MustKeep bool // pinned messages, tool pairs
}

// PackingResult contains the outcome of a context packing operation.
type PackingResult struct {
	KeptMessages    []int   // indices of kept messages
	DroppedMessages []int   // indices of dropped messages
	TotalTokens     int     // total tokens of kept messages
	Utilization     float64 // percentage of context used (0.0 - 1.0)
	Summary         string  // summary of dropped content
}

// NewContextPacker creates a new context packer for the given model context size.
func NewContextPacker(maxTokens int) *ContextPacker {
	return &ContextPacker{
		MaxTokens:          maxTokens,
		ReservedForOutput:  4096,
		SystemPromptTokens: 0,
		Strategy:           StrategyHybrid,
	}
}

// Pack scores each message and selects the optimal subset within budget.
// It ensures tool_use/tool_result pairs stay together, always keeps the system
// prompt context, first user message, and the last 4 messages.
func (cp *ContextPacker) Pack(messages []ScoredMessage, currentTask string) *PackingResult {
	if len(messages) == 0 {
		return &PackingResult{
			KeptMessages:    []int{},
			DroppedMessages: []int{},
			TotalTokens:     0,
			Utilization:     0,
			Summary:         "",
		}
	}

	budget := cp.MaxTokens - cp.ReservedForOutput - cp.SystemPromptTokens
	if budget < 0 {
		budget = 0
	}

	total := len(messages)

	// Score all messages.
	scored := make([]ScoredMessage, len(messages))
	copy(scored, messages)
	for i := range scored {
		if scored[i].Tokens == 0 {
			scored[i].Tokens = EstimateTokensFromContent(scored[i].Content)
		}
		scored[i].Score = cp.ScoreMessage(scored[i], currentTask, i, total)
	}

	// Mark must-keep messages: first user message, last 4 messages, pinned.
	cp.markMustKeep(scored)

	// Ensure tool pairs stay together.
	cp.linkToolPairs(scored)

	// Select optimal subset.
	kept := cp.OptimalSelection(scored, budget)

	// Build result.
	keptSet := make(map[int]bool, len(kept))
	for _, idx := range kept {
		keptSet[idx] = true
	}

	var dropped []int
	var droppedMsgs []ScoredMessage
	totalTokens := 0

	for i := range scored {
		if keptSet[i] {
			totalTokens += scored[i].Tokens
		} else {
			dropped = append(dropped, scored[i].Index)
			droppedMsgs = append(droppedMsgs, scored[i])
		}
	}

	sort.Ints(kept)
	sort.Ints(dropped)

	utilization := 0.0
	if budget > 0 {
		utilization = float64(totalTokens) / float64(budget)
		if utilization > 1.0 {
			utilization = 1.0
		}
	}

	summary := ""
	if len(droppedMsgs) > 0 {
		summary = SummarizeDropped(droppedMsgs)
	}

	return &PackingResult{
		KeptMessages:    kept,
		DroppedMessages: dropped,
		TotalTokens:     totalTokens,
		Utilization:     utilization,
		Summary:         summary,
	}
}

// ScoreMessage computes a composite score for a message based on multiple factors.
func (cp *ContextPacker) ScoreMessage(msg ScoredMessage, currentTask string, position int, total int) float64 {
	if msg.MustKeep {
		return 1000.0 // effectively infinite priority
	}

	var score float64

	switch cp.Strategy {
	case StrategyRecent:
		score = scoreRecency(position, total)
	case StrategyRelevance:
		score = scoreRelevance(msg.Content, currentTask)
	case StrategyCompression:
		// Compression strategy: high recency weight for recent, low for old.
		recency := scoreRecency(position, total)
		if recency > 0.7 {
			score = recency
		} else {
			score = recency * 0.3
		}
	default: // StrategyHybrid
		recency := scoreRecency(position, total)
		relevance := scoreRelevance(msg.Content, currentTask)
		roleScore := scoreRole(msg.Role)
		toolScore := scoreToolContent(msg.Content, msg.Role)
		lengthPenalty := scoreLengthPenalty(msg.Tokens)

		// Weighted combination.
		score = recency*0.4 + relevance*0.3 + roleScore*0.1 + toolScore*0.1 + lengthPenalty*0.1
	}

	return score
}

// OptimalSelection performs greedy selection by score/token ratio,
// respecting constraints (tool pairs, pinned messages).
func (cp *ContextPacker) OptimalSelection(messages []ScoredMessage, budget int) []int {
	if len(messages) == 0 {
		return []int{}
	}

	// First pass: include all must-keep messages.
	var selected []int
	remaining := budget
	included := make(map[int]bool)

	for i, msg := range messages {
		if msg.MustKeep {
			selected = append(selected, msg.Index)
			included[i] = true
			remaining -= msg.Tokens
		}
	}

	// If must-keep already exceeds budget, return just those.
	if remaining <= 0 {
		return selected
	}

	// Build candidates sorted by score/token ratio (value density).
	type candidate struct {
		idx   int
		ratio float64
	}
	var candidates []candidate
	for i, msg := range messages {
		if included[i] {
			continue
		}
		tokens := msg.Tokens
		if tokens <= 0 {
			tokens = 1
		}
		ratio := msg.Score / float64(tokens)
		candidates = append(candidates, candidate{idx: i, ratio: ratio})
	}

	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].ratio > candidates[b].ratio
	})

	// Greedy fill.
	for _, c := range candidates {
		msg := messages[c.idx]
		if msg.Tokens <= remaining {
			selected = append(selected, msg.Index)
			included[c.idx] = true
			remaining -= msg.Tokens
		}
	}

	return selected
}

// EstimateTokensFromContent provides a quick token estimate for content.
// Uses len/4 for English text, len/3 for code-heavy content.
func EstimateTokensFromContent(content string) int {
	if len(content) == 0 {
		return 0
	}

	// Heuristic: if content has many code indicators, use len/3.
	codeIndicators := 0
	indicators := []string{"{", "}", "()", "func ", "def ", "class ", "import ", "var ", ":=", "->", "=>"}
	lower := strings.ToLower(content)
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			codeIndicators++
		}
	}

	if codeIndicators >= 3 {
		return len(content) / 3
	}
	return len(content) / 4
}

// SummarizeDropped creates a brief summary of what was dropped.
func SummarizeDropped(dropped []ScoredMessage) string {
	if len(dropped) == 0 {
		return ""
	}

	toolCalls := 0
	userMsgs := 0
	assistantMsgs := 0
	var topics []string
	seenTopics := make(map[string]bool)

	for _, msg := range dropped {
		switch msg.Role {
		case "user":
			userMsgs++
		case "assistant":
			assistantMsgs++
		case "tool", "tool_result":
			toolCalls++
		}

		// Extract topic hints from content (first few meaningful words).
		topic := extractTopic(msg.Content)
		if topic != "" && !seenTopics[topic] {
			seenTopics[topic] = true
			if len(topics) < 5 {
				topics = append(topics, topic)
			}
		}
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Earlier: %d messages dropped", len(dropped)))

	if toolCalls > 0 {
		parts = append(parts, fmt.Sprintf("%d tool calls", toolCalls))
	}
	if userMsgs > 0 {
		parts = append(parts, fmt.Sprintf("%d user messages", userMsgs))
	}
	if assistantMsgs > 0 {
		parts = append(parts, fmt.Sprintf("%d assistant messages", assistantMsgs))
	}

	if len(topics) > 0 {
		parts = append(parts, "topics: "+strings.Join(topics, ", "))
	}

	return strings.Join(parts, "; ")
}

// PackingReport generates a human-readable report of the packing result.
func PackingReport(result *PackingResult, strategy PackingStrategy, totalMessages int, mustKeepCount int) string {
	if result == nil {
		return "Context Packing: no result"
	}

	budget := 0
	if result.Utilization > 0 {
		budget = int(float64(result.TotalTokens) / result.Utilization)
	}

	var sb strings.Builder
	sb.WriteString("Context Packing:\n")
	sb.WriteString(fmt.Sprintf("  Kept: %d/%d messages (%d/%d tokens, %.0f%%)\n",
		len(result.KeptMessages), totalMessages,
		result.TotalTokens, budget,
		result.Utilization*100))
	sb.WriteString(fmt.Sprintf("  Dropped: %d messages (summarized)\n", len(result.DroppedMessages)))
	sb.WriteString(fmt.Sprintf("  Strategy: %s\n", strategy))
	sb.WriteString(fmt.Sprintf("  Must-keep: %d (pinned + tool pairs)\n", mustKeepCount))

	return sb.String()
}

// markMustKeep marks messages that must always be kept:
// - First user message
// - Last 4 messages
// - Already-pinned messages
func (cp *ContextPacker) markMustKeep(messages []ScoredMessage) {
	if len(messages) == 0 {
		return
	}

	// First user message.
	for i := range messages {
		if messages[i].Role == "user" {
			messages[i].MustKeep = true
			break
		}
	}

	// Last 4 messages.
	start := len(messages) - 4
	if start < 0 {
		start = 0
	}
	for i := start; i < len(messages); i++ {
		messages[i].MustKeep = true
	}
}

// linkToolPairs ensures that tool_use and tool_result messages stay together.
// If one is must-keep, the paired message becomes must-keep too.
func (cp *ContextPacker) linkToolPairs(messages []ScoredMessage) {
	for i := range messages {
		if !messages[i].MustKeep {
			continue
		}

		// If this is a tool result, keep the preceding assistant message.
		if messages[i].Role == "tool_result" || messages[i].Role == "tool" {
			if i > 0 && messages[i-1].Role == "assistant" {
				messages[i-1].MustKeep = true
			}
		}

		// If this is an assistant message with tool use indication, keep the next tool result.
		if messages[i].Role == "assistant" {
			if i+1 < len(messages) && (messages[i+1].Role == "tool_result" || messages[i+1].Role == "tool") {
				messages[i+1].MustKeep = true
			}
		}
	}
}

// scoreRecency computes an exponential decay score based on position.
// Newer messages (higher position) get higher scores.
func scoreRecency(position, total int) float64 {
	if total <= 1 {
		return 1.0
	}
	// Normalized position: 0.0 (oldest) to 1.0 (newest).
	normalized := float64(position) / float64(total-1)
	// Exponential decay: e^(-3 * (1 - normalized))
	return math.Exp(-3.0 * (1.0 - normalized))
}

// scoreRelevance computes keyword overlap between content and currentTask.
func scoreRelevance(content, currentTask string) float64 {
	if currentTask == "" || content == "" {
		return 0.0
	}

	taskWords := tokenizeForRelevance(currentTask)
	if len(taskWords) == 0 {
		return 0.0
	}

	contentLower := strings.ToLower(content)
	matches := 0
	for _, word := range taskWords {
		if strings.Contains(contentLower, word) {
			matches++
		}
	}

	score := float64(matches) / float64(len(taskWords))
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// scoreRole returns a score based on message role.
// User messages score slightly higher than assistant messages.
func scoreRole(role string) float64 {
	switch role {
	case "user":
		return 0.7
	case "assistant":
		return 0.5
	case "tool_result", "tool":
		return 0.6
	default:
		return 0.4
	}
}

// scoreToolContent gives higher scores to tool results with errors.
func scoreToolContent(content, role string) float64 {
	if role != "tool_result" && role != "tool" {
		return 0.5
	}

	lower := strings.ToLower(content)
	errorIndicators := []string{"error", "failed", "panic", "exception", "traceback", "fatal"}
	for _, ind := range errorIndicators {
		if strings.Contains(lower, ind) {
			return 1.0
		}
	}
	return 0.4
}

// scoreLengthPenalty applies a slight penalty to very long messages.
func scoreLengthPenalty(tokens int) float64 {
	if tokens <= 100 {
		return 1.0
	}
	if tokens <= 500 {
		return 0.8
	}
	if tokens <= 2000 {
		return 0.6
	}
	return 0.4
}

// tokenizeForRelevance splits text into lowercase words for relevance matching.
func tokenizeForRelevance(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	// Filter out very short/common words.
	var result []string
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "in": true,
		"to": true, "for": true, "of": true, "and": true, "or": true,
		"it": true, "on": true, "at": true, "by": true, "with": true,
		"this": true, "that": true, "from": true, "as": true, "be": true,
	}
	for _, w := range words {
		if len(w) >= 3 && !stopWords[w] {
			result = append(result, w)
		}
	}
	return result
}

// extractTopic extracts a brief topic hint from message content.
func extractTopic(content string) string {
	if len(content) == 0 {
		return ""
	}

	// Take first line or first 50 chars.
	line := content
	if idx := strings.IndexByte(content, '\n'); idx > 0 {
		line = content[:idx]
	}
	if len(line) > 50 {
		line = line[:50]
	}

	line = strings.TrimSpace(line)
	if len(line) < 5 {
		return ""
	}
	return line
}
