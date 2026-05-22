package ctxmgr

import (
	"math"
	"strings"
	"testing"
)

func TestPack_KeepsRecentMessages(t *testing.T) {
	cp := NewContextPacker(1000)
	cp.Strategy = StrategyRecent

	messages := make([]ScoredMessage, 10)
	for i := range messages {
		messages[i] = ScoredMessage{
			Index:   i,
			Role:    "user",
			Content: strings.Repeat("word ", 20), // ~25 tokens each
			Tokens:  25,
		}
	}

	result := cp.Pack(messages, "")

	// Last 4 messages should always be kept.
	keptSet := make(map[int]bool)
	for _, idx := range result.KeptMessages {
		keptSet[idx] = true
	}
	for i := 6; i < 10; i++ {
		if !keptSet[i] {
			t.Errorf("expected message %d (recent) to be kept", i)
		}
	}
}

func TestPack_KeepsToolPairsTogether(t *testing.T) {
	cp := NewContextPacker(2000)
	cp.Strategy = StrategyHybrid

	messages := []ScoredMessage{
		{Index: 0, Role: "user", Content: "please read the file", Tokens: 10},
		{Index: 1, Role: "assistant", Content: "I will read it", Tokens: 10},
		{Index: 2, Role: "tool_result", Content: "file contents here", Tokens: 10},
		{Index: 3, Role: "user", Content: "now do something", Tokens: 10},
		{Index: 4, Role: "assistant", Content: "doing it", Tokens: 10},
		{Index: 5, Role: "tool_result", Content: "done", Tokens: 10},
		{Index: 6, Role: "user", Content: "last message one", Tokens: 10},
		{Index: 7, Role: "assistant", Content: "last message two", Tokens: 10},
		{Index: 8, Role: "user", Content: "last message three", Tokens: 10},
		{Index: 9, Role: "assistant", Content: "last message four", Tokens: 10},
	}

	result := cp.Pack(messages, "read file")

	keptSet := make(map[int]bool)
	for _, idx := range result.KeptMessages {
		keptSet[idx] = true
	}

	// If a tool_result is kept, its preceding assistant should be too.
	for _, idx := range result.KeptMessages {
		if idx > 0 && idx < len(messages) {
			if messages[idx].Role == "tool_result" {
				if !keptSet[idx-1] {
					t.Errorf("tool_result at %d is kept but paired assistant at %d is not", idx, idx-1)
				}
			}
		}
	}
}

func TestPack_RespectsBudget(t *testing.T) {
	cp := NewContextPacker(500)
	cp.ReservedForOutput = 100
	cp.SystemPromptTokens = 50
	// Budget = 500 - 100 - 50 = 350 tokens

	messages := make([]ScoredMessage, 20)
	for i := range messages {
		messages[i] = ScoredMessage{
			Index:   i,
			Role:    "user",
			Content: strings.Repeat("x ", 50),
			Tokens:  50,
		}
	}

	result := cp.Pack(messages, "")

	// Total tokens should not exceed budget of 350.
	if result.TotalTokens > 350 {
		t.Errorf("total tokens %d exceeds budget 350", result.TotalTokens)
	}

	// Should have dropped some messages.
	if len(result.DroppedMessages) == 0 {
		t.Error("expected some messages to be dropped")
	}
}

func TestScoreMessage_RecencyDecay(t *testing.T) {
	cp := NewContextPacker(8000)
	cp.Strategy = StrategyHybrid

	total := 20
	oldMsg := ScoredMessage{Index: 0, Role: "user", Content: "old message", Tokens: 10}
	newMsg := ScoredMessage{Index: 19, Role: "user", Content: "new message", Tokens: 10}

	oldScore := cp.ScoreMessage(oldMsg, "task", 0, total)
	newScore := cp.ScoreMessage(newMsg, "task", 19, total)

	if newScore <= oldScore {
		t.Errorf("newer message should score higher: old=%.4f, new=%.4f", oldScore, newScore)
	}
}

func TestScoreMessage_RelevanceBoost(t *testing.T) {
	cp := NewContextPacker(8000)
	cp.Strategy = StrategyRelevance

	relevantMsg := ScoredMessage{
		Index:   5,
		Role:    "user",
		Content: "implement authentication with JWT tokens and OAuth",
		Tokens:  15,
	}
	irrelevantMsg := ScoredMessage{
		Index:   5,
		Role:    "user",
		Content: "the weather is nice today",
		Tokens:  10,
	}

	task := "implement authentication system"
	relevantScore := cp.ScoreMessage(relevantMsg, task, 5, 10)
	irrelevantScore := cp.ScoreMessage(irrelevantMsg, task, 5, 10)

	if relevantScore <= irrelevantScore {
		t.Errorf("relevant message should score higher: relevant=%.4f, irrelevant=%.4f",
			relevantScore, irrelevantScore)
	}
}

func TestOptimalSelection_Greedy(t *testing.T) {
	cp := NewContextPacker(8000)

	messages := []ScoredMessage{
		{Index: 0, Role: "user", Content: "hi", Tokens: 100, Score: 0.9, MustKeep: false},
		{Index: 1, Role: "user", Content: strings.Repeat("x", 400), Tokens: 400, Score: 0.5, MustKeep: false},
		{Index: 2, Role: "user", Content: "important", Tokens: 50, Score: 0.8, MustKeep: false},
		{Index: 3, Role: "user", Content: strings.Repeat("y", 300), Tokens: 300, Score: 0.3, MustKeep: false},
	}

	// Budget of 200: should pick messages with best score/token ratio.
	selected := cp.OptimalSelection(messages, 200)

	selectedSet := make(map[int]bool)
	for _, idx := range selected {
		selectedSet[idx] = true
	}

	// Message 2 (score 0.8, tokens 50, ratio 0.016) and message 0 (score 0.9, tokens 100, ratio 0.009)
	// should be preferred over message 1 (ratio 0.00125) and message 3 (ratio 0.001).
	if !selectedSet[2] {
		t.Error("expected message 2 (high ratio) to be selected")
	}
	if !selectedSet[0] {
		t.Error("expected message 0 (high ratio) to be selected")
	}
}

func TestSummarizeDropped_Output(t *testing.T) {
	dropped := []ScoredMessage{
		{Index: 0, Role: "user", Content: "implement auth system", Tokens: 20},
		{Index: 1, Role: "assistant", Content: "I will implement it", Tokens: 15},
		{Index: 2, Role: "tool_result", Content: "file read successfully", Tokens: 30},
		{Index: 3, Role: "user", Content: "now add tests", Tokens: 10},
		{Index: 4, Role: "tool_result", Content: "test results passed", Tokens: 25},
	}

	summary := SummarizeDropped(dropped)

	if !strings.Contains(summary, "5 messages dropped") {
		t.Errorf("summary should mention dropped count, got: %s", summary)
	}
	if !strings.Contains(summary, "tool calls") {
		t.Errorf("summary should mention tool calls, got: %s", summary)
	}
	if !strings.Contains(summary, "user messages") {
		t.Errorf("summary should mention user messages, got: %s", summary)
	}
}

func TestPack_PinnedMessagesAlwaysKept(t *testing.T) {
	cp := NewContextPacker(300)
	cp.ReservedForOutput = 50
	cp.SystemPromptTokens = 0
	// Budget = 250 tokens

	messages := []ScoredMessage{
		{Index: 0, Role: "user", Content: "first message", Tokens: 50, MustKeep: true},
		{Index: 1, Role: "user", Content: "pinned important", Tokens: 50, MustKeep: true},
		{Index: 2, Role: "user", Content: "normal message 1", Tokens: 50},
		{Index: 3, Role: "user", Content: "normal message 2", Tokens: 50},
		{Index: 4, Role: "user", Content: "normal message 3", Tokens: 50},
		{Index: 5, Role: "user", Content: "normal message 4", Tokens: 50},
	}

	result := cp.Pack(messages, "")

	keptSet := make(map[int]bool)
	for _, idx := range result.KeptMessages {
		keptSet[idx] = true
	}

	// Pinned messages must be kept.
	if !keptSet[0] {
		t.Error("pinned message 0 should always be kept")
	}
	if !keptSet[1] {
		t.Error("pinned message 1 should always be kept")
	}
}

func TestPack_FirstAndLastMessagesPreserved(t *testing.T) {
	cp := NewContextPacker(2000)
	cp.ReservedForOutput = 100

	messages := make([]ScoredMessage, 10)
	for i := range messages {
		messages[i] = ScoredMessage{
			Index:   i,
			Role:    "user",
			Content: "message content",
			Tokens:  30,
		}
	}

	result := cp.Pack(messages, "")

	keptSet := make(map[int]bool)
	for _, idx := range result.KeptMessages {
		keptSet[idx] = true
	}

	// First user message (index 0) should be kept.
	if !keptSet[0] {
		t.Error("first user message should be preserved")
	}

	// Last 4 messages (indices 6-9) should be kept.
	for i := 6; i < 10; i++ {
		if !keptSet[i] {
			t.Errorf("last-4 message %d should be preserved", i)
		}
	}
}

func TestPack_UtilizationCalculation(t *testing.T) {
	cp := NewContextPacker(1000)
	cp.ReservedForOutput = 0
	cp.SystemPromptTokens = 0
	// Budget = 1000 tokens

	messages := []ScoredMessage{
		{Index: 0, Role: "user", Content: "hello", Tokens: 200},
		{Index: 1, Role: "user", Content: "world", Tokens: 300},
	}

	result := cp.Pack(messages, "")

	// Both messages fit (500 tokens < 1000 budget).
	expectedUtilization := 500.0 / 1000.0
	if math.Abs(result.Utilization-expectedUtilization) > 0.01 {
		t.Errorf("expected utilization %.2f, got %.2f", expectedUtilization, result.Utilization)
	}
}

func TestPack_StrategySwitching(t *testing.T) {
	strategies := []PackingStrategy{
		StrategyRecent,
		StrategyRelevance,
		StrategyHybrid,
		StrategyCompression,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			cp := NewContextPacker(2000)
			cp.Strategy = strategy

			messages := make([]ScoredMessage, 8)
			for i := range messages {
				messages[i] = ScoredMessage{
					Index:   i,
					Role:    "user",
					Content: "some content here",
					Tokens:  30,
				}
			}

			result := cp.Pack(messages, "current task")

			if result == nil {
				t.Fatal("Pack returned nil result")
			}
			if len(result.KeptMessages) == 0 {
				t.Error("should keep at least some messages")
			}
		})
	}
}

func TestPack_EmptyMessagesList(t *testing.T) {
	cp := NewContextPacker(8000)

	result := cp.Pack([]ScoredMessage{}, "task")

	if result == nil {
		t.Fatal("Pack returned nil for empty messages")
	}
	if len(result.KeptMessages) != 0 {
		t.Errorf("expected 0 kept messages, got %d", len(result.KeptMessages))
	}
	if len(result.DroppedMessages) != 0 {
		t.Errorf("expected 0 dropped messages, got %d", len(result.DroppedMessages))
	}
	if result.TotalTokens != 0 {
		t.Errorf("expected 0 total tokens, got %d", result.TotalTokens)
	}
	if result.Utilization != 0 {
		t.Errorf("expected 0 utilization, got %f", result.Utilization)
	}
}

func TestEstimateTokensFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		minTok  int
		maxTok  int
	}{
		{
			name:    "empty",
			content: "",
			minTok:  0,
			maxTok:  0,
		},
		{
			name:    "english text",
			content: "This is a simple English sentence with about ten words here.",
			minTok:  10,
			maxTok:  20,
		},
		{
			name:    "code content",
			content: "func main() {\n\tfmt.Println(\"hello\")\n\tvar x := 5\n\tif x > 0 {\n\t\treturn x\n\t}\n}",
			minTok:  20,
			maxTok:  40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := EstimateTokensFromContent(tt.content)
			if tokens < tt.minTok || tokens > tt.maxTok {
				t.Errorf("EstimateTokensFromContent(%q) = %d, want in [%d, %d]",
					tt.content, tokens, tt.minTok, tt.maxTok)
			}
		})
	}
}

func TestScoreRecency_ExponentialDecay(t *testing.T) {
	total := 10

	scores := make([]float64, total)
	for i := 0; i < total; i++ {
		scores[i] = scoreRecency(i, total)
	}

	// Scores should be monotonically increasing.
	for i := 1; i < total; i++ {
		if scores[i] <= scores[i-1] {
			t.Errorf("recency scores should increase: position %d (%.4f) <= position %d (%.4f)",
				i, scores[i], i-1, scores[i-1])
		}
	}

	// Last message should score close to 1.0.
	if scores[total-1] < 0.9 {
		t.Errorf("last message recency should be near 1.0, got %.4f", scores[total-1])
	}

	// First message should be significantly lower.
	if scores[0] > 0.1 {
		t.Errorf("first message recency should be low, got %.4f", scores[0])
	}
}

func TestScoreRelevance_KeywordOverlap(t *testing.T) {
	task := "implement authentication with JWT"

	// High overlap.
	high := scoreRelevance("authentication system using JWT tokens", task)
	// No overlap.
	none := scoreRelevance("the weather forecast for today", task)
	// Partial overlap.
	partial := scoreRelevance("implement the new feature", task)

	if high <= partial {
		t.Errorf("high overlap (%.4f) should score > partial (%.4f)", high, partial)
	}
	if partial <= none {
		t.Errorf("partial overlap (%.4f) should score > none (%.4f)", partial, none)
	}
	if none != 0.0 {
		// "the" is a stop word, "weather", "forecast", "today" don't match task keywords.
		// This might be 0 depending on stopword filtering.
		// Actually let's just check it's lower than partial.
		if none >= partial {
			t.Errorf("no overlap (%.4f) should score < partial (%.4f)", none, partial)
		}
	}
}

func TestPackingReport_Format(t *testing.T) {
	result := &PackingResult{
		KeptMessages:    []int{0, 1, 2, 5, 8, 9},
		DroppedMessages: []int{3, 4, 6, 7},
		TotalTokens:     4200,
		Utilization:     0.525,
		Summary:         "Earlier context",
	}

	report := PackingReport(result, StrategyHybrid, 10, 3)

	if !strings.Contains(report, "Context Packing:") {
		t.Error("report should contain header")
	}
	if !strings.Contains(report, "6/10") {
		t.Error("report should show kept/total messages")
	}
	if !strings.Contains(report, "hybrid") {
		t.Error("report should show strategy")
	}
	if !strings.Contains(report, "Must-keep: 3") {
		t.Error("report should show must-keep count")
	}
	if !strings.Contains(report, "Dropped: 4") {
		t.Error("report should show dropped count")
	}
}

func TestNewContextPacker_Defaults(t *testing.T) {
	cp := NewContextPacker(128000)

	if cp.MaxTokens != 128000 {
		t.Errorf("expected MaxTokens 128000, got %d", cp.MaxTokens)
	}
	if cp.ReservedForOutput != 4096 {
		t.Errorf("expected ReservedForOutput 4096, got %d", cp.ReservedForOutput)
	}
	if cp.Strategy != StrategyHybrid {
		t.Errorf("expected default strategy hybrid, got %s", cp.Strategy)
	}
}

func TestOptimalSelection_MustKeepExceedsBudget(t *testing.T) {
	cp := NewContextPacker(8000)

	messages := []ScoredMessage{
		{Index: 0, Role: "user", Content: "big pinned", Tokens: 100, Score: 1000, MustKeep: true},
		{Index: 1, Role: "user", Content: "another pinned", Tokens: 100, Score: 1000, MustKeep: true},
		{Index: 2, Role: "user", Content: "optional", Tokens: 50, Score: 0.5, MustKeep: false},
	}

	// Budget smaller than must-keep total.
	selected := cp.OptimalSelection(messages, 150)

	selectedSet := make(map[int]bool)
	for _, idx := range selected {
		selectedSet[idx] = true
	}

	// Must-keep messages should still be included even if over budget.
	if !selectedSet[0] {
		t.Error("must-keep message 0 should be selected even over budget")
	}
	if !selectedSet[1] {
		t.Error("must-keep message 1 should be selected even over budget")
	}
}

func TestSummarizeDropped_Empty(t *testing.T) {
	summary := SummarizeDropped(nil)
	if summary != "" {
		t.Errorf("expected empty summary for nil input, got: %s", summary)
	}

	summary = SummarizeDropped([]ScoredMessage{})
	if summary != "" {
		t.Errorf("expected empty summary for empty input, got: %s", summary)
	}
}

func TestScoreToolContent_ErrorsScoreHigher(t *testing.T) {
	errorResult := scoreToolContent("Error: file not found", "tool_result")
	normalResult := scoreToolContent("file contents here", "tool_result")
	nonTool := scoreToolContent("Error: something", "user")

	if errorResult <= normalResult {
		t.Errorf("error tool result (%.2f) should score > normal (%.2f)", errorResult, normalResult)
	}
	if nonTool != 0.5 {
		t.Errorf("non-tool message should get 0.5, got %.2f", nonTool)
	}
}

func TestScoreLengthPenalty(t *testing.T) {
	short := scoreLengthPenalty(50)
	medium := scoreLengthPenalty(300)
	long := scoreLengthPenalty(1500)
	veryLong := scoreLengthPenalty(5000)

	if short <= medium {
		t.Errorf("short (%.2f) should score > medium (%.2f)", short, medium)
	}
	if medium <= long {
		t.Errorf("medium (%.2f) should score > long (%.2f)", medium, long)
	}
	if long <= veryLong {
		t.Errorf("long (%.2f) should score > veryLong (%.2f)", long, veryLong)
	}
}
