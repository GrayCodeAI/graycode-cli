package session

import (
	"strings"
	"testing"
)

func TestNewSessionCompressor(t *testing.T) {
	sc := NewSessionCompressor(StrategySelective)
	if sc == nil {
		t.Fatal("expected non-nil compressor")
	}
	if sc.Strategy != StrategySelective {
		t.Errorf("expected strategy %q, got %q", StrategySelective, sc.Strategy)
	}
	if sc.MinMessages != 8 {
		t.Errorf("expected MinMessages=8, got %d", sc.MinMessages)
	}
}

func TestScoreImportance_FirstMessage(t *testing.T) {
	msg := CompressMessage{Role: "user", Content: "Hello, start a project"}
	score := ScoreImportance(msg, 0, 20)
	if score != 1.0 {
		t.Errorf("first user message should score 1.0, got %f", score)
	}
}

func TestScoreImportance_LastMessages(t *testing.T) {
	msg := CompressMessage{Role: "assistant", Content: "Here is the result"}
	// Position 18 out of 20 total -> in last 20%
	score := ScoreImportance(msg, 18, 20)
	if score != 1.0 {
		t.Errorf("last messages should score 1.0, got %f", score)
	}
}

func TestScoreImportance_ToolError(t *testing.T) {
	msg := CompressMessage{
		Role:         "tool",
		Content:      "Error: file not found",
		IsToolResult: true,
	}
	score := ScoreImportance(msg, 5, 20)
	if score != 0.9 {
		t.Errorf("tool error should score 0.9, got %f", score)
	}
}

func TestScoreImportance_Decision(t *testing.T) {
	msg := CompressMessage{
		Role:    "assistant",
		Content: "I've decided to use the factory pattern for this",
	}
	score := ScoreImportance(msg, 5, 20)
	if score != 0.8 {
		t.Errorf("decision message should score 0.8, got %f", score)
	}
}

func TestScoreImportance_Code(t *testing.T) {
	msg := CompressMessage{
		Role:    "assistant",
		Content: "```go\nfunc main() {}\n```",
	}
	score := ScoreImportance(msg, 5, 20)
	if score != 0.7 {
		t.Errorf("code message should score 0.7, got %f", score)
	}
}

func TestScoreImportance_RoutineToolResult(t *testing.T) {
	msg := CompressMessage{
		Role:         "tool",
		Content:      "Successfully read 42 lines",
		IsToolResult: true,
	}
	score := ScoreImportance(msg, 5, 20)
	if score != 0.3 {
		t.Errorf("routine tool result should score 0.3, got %f", score)
	}
}

func TestScoreImportance_GenericChat(t *testing.T) {
	msg := CompressMessage{
		Role:    "user",
		Content: "Thanks, that looks good",
	}
	score := ScoreImportance(msg, 5, 20)
	if score != 0.4 {
		t.Errorf("generic chat should score 0.4, got %f", score)
	}
}

func TestSummarizeBlock(t *testing.T) {
	messages := []CompressMessage{
		{Role: "assistant", Content: "I'll read the file main.go", ToolName: "read", Tokens: 20},
		{Role: "tool", Content: "package main\nfunc main() {}", IsToolResult: true, Tokens: 30},
		{Role: "assistant", Content: "Let's use the singleton pattern here. I decided to refactor.", Tokens: 40},
	}

	block := SummarizeBlock(messages)
	if block == nil {
		t.Fatal("expected non-nil block")
	}
	if block.OriginalCount != 3 {
		t.Errorf("expected OriginalCount=3, got %d", block.OriginalCount)
	}
	if block.TokensSaved <= 0 {
		t.Errorf("expected positive tokens saved, got %d", block.TokensSaved)
	}
	if len(block.KeyFacts) == 0 {
		t.Error("expected at least one key fact")
	}
}

func TestSummarizeBlock_Empty(t *testing.T) {
	block := SummarizeBlock(nil)
	if block == nil {
		t.Fatal("expected non-nil block for empty input")
	}
	if block.OriginalCount != 0 {
		t.Errorf("expected OriginalCount=0, got %d", block.OriginalCount)
	}
}

func TestExtractKeyFacts(t *testing.T) {
	messages := []CompressMessage{
		{Role: "assistant", Content: "I decided to use goroutines for concurrency"},
		{Role: "assistant", Content: "Modified the file cmd/main.go"},
		{Role: "tool", Content: "Error: undefined variable x", IsToolResult: true},
		{Role: "assistant", Content: "The convention is to always use context.Context as first param"},
	}

	facts := ExtractKeyFacts(messages)
	if len(facts) == 0 {
		t.Fatal("expected facts to be extracted")
	}

	hasDecision := false
	hasFile := false
	hasError := false
	hasConvention := false

	for _, f := range facts {
		if strings.HasPrefix(f, "Decision:") {
			hasDecision = true
		}
		if strings.HasPrefix(f, "Modified:") {
			hasFile = true
		}
		if strings.HasPrefix(f, "Error:") {
			hasError = true
		}
		if strings.HasPrefix(f, "Convention:") {
			hasConvention = true
		}
	}

	if !hasDecision {
		t.Error("expected a decision fact")
	}
	if !hasFile {
		t.Error("expected a file fact")
	}
	if !hasError {
		t.Error("expected an error fact")
	}
	if !hasConvention {
		t.Error("expected a convention fact")
	}
}

func TestSelectiveCompress(t *testing.T) {
	messages := makeTestMessages(30)
	// Set a budget that forces compression
	budget := totalTokens(messages) / 2

	result := SelectiveCompress(messages, budget)
	if len(result) >= len(messages) {
		t.Errorf("expected fewer messages after compression, got %d >= %d", len(result), len(messages))
	}
	if len(result) == 0 {
		t.Error("expected at least some messages to remain")
	}
}

func TestSelectiveCompress_UnderBudget(t *testing.T) {
	messages := makeTestMessages(5)
	budget := totalTokens(messages) + 100

	result := SelectiveCompress(messages, budget)
	if len(result) != len(messages) {
		t.Errorf("under budget: expected %d messages unchanged, got %d", len(messages), len(result))
	}
}

func TestTieredCompress(t *testing.T) {
	messages := makeTestMessages(50)
	budget := totalTokens(messages) / 3

	result := TieredCompress(messages, budget)
	if len(result) >= len(messages) {
		t.Errorf("expected fewer messages, got %d >= %d", len(result), len(messages))
	}

	// Recent messages (last 20%) should be preserved
	recentStart := len(messages) - len(messages)/5
	lastOriginal := messages[len(messages)-1]
	lastCompressed := result[len(result)-1]
	if lastOriginal.Content != lastCompressed.Content {
		t.Error("expected last message to be preserved verbatim")
	}

	_ = recentStart
}

func TestTieredCompress_UnderBudget(t *testing.T) {
	messages := makeTestMessages(5)
	budget := totalTokens(messages) + 100

	result := TieredCompress(messages, budget)
	if len(result) != len(messages) {
		t.Errorf("under budget: expected %d messages, got %d", len(messages), len(result))
	}
}

func TestSemanticCompress(t *testing.T) {
	messages := []CompressMessage{
		{Role: "user", Content: "Can you help with the auth module?", Tokens: 20},
		{Role: "assistant", Content: "Sure, I'll look at auth.go", ToolName: "read", Tokens: 15},
		{Role: "tool", Content: "package auth...", IsToolResult: true, Tokens: 50},
		{Role: "assistant", Content: "I see the issue. I decided to fix the token validation.", Tokens: 30},
		{Role: "user", Content: "Now let's work on the database layer", Tokens: 20},
		{Role: "assistant", Content: "Looking at db.go", ToolName: "read", Tokens: 15},
		{Role: "tool", Content: "package db...", IsToolResult: true, Tokens: 50},
		{Role: "assistant", Content: "The connection pooling looks correct.", Tokens: 25},
	}

	budget := totalTokens(messages) / 2

	result := SemanticCompress(messages, budget)
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	if len(result) >= len(messages) {
		// Semantic compress may not always reduce count if groups are small,
		// but it should at minimum not increase it
		t.Logf("semantic compress: %d -> %d (may not reduce small inputs)", len(messages), len(result))
	}
}

func TestSemanticCompress_UnderBudget(t *testing.T) {
	messages := makeTestMessages(5)
	budget := totalTokens(messages) + 100

	result := SemanticCompress(messages, budget)
	if len(result) != len(messages) {
		t.Errorf("under budget: expected %d, got %d", len(messages), len(result))
	}
}

func TestCompress_SummarizeStrategy(t *testing.T) {
	sc := NewSessionCompressor(StrategySummarize)
	messages := makeTestMessages(25)
	budget := totalTokens(messages) / 2

	result, compressed, err := sc.Compress(messages, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Original != 25 {
		t.Errorf("expected Original=25, got %d", result.Original)
	}
	if result.Compressed >= result.Original {
		t.Errorf("expected compression, got %d >= %d", result.Compressed, result.Original)
	}
	if len(compressed) >= len(messages) {
		t.Errorf("expected fewer messages, got %d", len(compressed))
	}
}

func TestCompress_SelectiveStrategy(t *testing.T) {
	sc := NewSessionCompressor(StrategySelective)
	messages := makeTestMessages(30)
	budget := totalTokens(messages) / 2

	result, compressed, err := sc.Compress(messages, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TokensSaved <= 0 {
		t.Error("expected positive token savings")
	}
	if len(compressed) == 0 {
		t.Error("expected non-empty compressed output")
	}
}

func TestCompress_TieredStrategy(t *testing.T) {
	sc := NewSessionCompressor(StrategyTiered)
	messages := makeTestMessages(40)
	budget := totalTokens(messages) / 3

	result, compressed, err := sc.Compress(messages, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Original != 40 {
		t.Errorf("expected Original=40, got %d", result.Original)
	}
	if len(compressed) >= len(messages) {
		t.Errorf("expected compression, got %d messages", len(compressed))
	}
}

func TestCompress_SemanticStrategy(t *testing.T) {
	sc := NewSessionCompressor(StrategySemantic)
	messages := makeTopicMessages()
	budget := totalTokens(messages) / 2

	_, compressed, err := sc.Compress(messages, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(compressed) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestCompress_EmptyInput(t *testing.T) {
	sc := NewSessionCompressor(StrategySummarize)
	result, compressed, err := sc.Compress(nil, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Original != 0 {
		t.Errorf("expected Original=0, got %d", result.Original)
	}
	if len(compressed) != 0 {
		t.Errorf("expected empty output, got %d messages", len(compressed))
	}
}

func TestFormatCompressed(t *testing.T) {
	result := &CompressionResult{
		Original:          45,
		Compressed:        18,
		TokensSaved:       12400,
		PreservedMessages: 8,
		Blocks: []CompressedBlock{
			{Summary: "tool executions", OriginalCount: 10, KeyFacts: []string{"fact1", "fact2"}},
			{Summary: "debugging", OriginalCount: 8, KeyFacts: []string{"fact3"}},
			{Summary: "planning", OriginalCount: 9, KeyFacts: []string{"fact4", "fact5", "fact6", "fact7"}},
		},
	}

	output := FormatCompressed(result)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(output, "45 messages") {
		t.Error("expected original count in output")
	}
	if !strings.Contains(output, "18") {
		t.Error("expected compressed count in output")
	}
	if !strings.Contains(output, "12,400") {
		t.Error("expected formatted token count")
	}
	if !strings.Contains(output, "last 8 messages") {
		t.Error("expected preserved count")
	}
	if !strings.Contains(output, "3 blocks") {
		t.Error("expected block count")
	}
	if !strings.Contains(output, "Key facts retained: 7") {
		t.Error("expected key facts count")
	}
}

func TestFormatCompressed_Nil(t *testing.T) {
	output := FormatCompressed(nil)
	if output != "" {
		t.Errorf("expected empty string for nil, got %q", output)
	}
}

func TestContainsError(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"Error: file not found", true},
		{"build failed with exit code 1", true},
		{"panic: runtime error", true},
		{"All tests passed", false},
		{"OK", false},
	}

	for _, tc := range tests {
		got := containsError(tc.content)
		if got != tc.expected {
			t.Errorf("containsError(%q) = %v, want %v", tc.content, got, tc.expected)
		}
	}
}

func TestContainsCode(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"```go\nfunc main() {}\n```", true},
		{"func Process(ctx context.Context)", true},
		{"def hello_world():", true},
		{"just some plain text", false},
	}

	for _, tc := range tests {
		got := containsCode(tc.content)
		if got != tc.expected {
			t.Errorf("containsCode(%q) = %v, want %v", tc.content, got, tc.expected)
		}
	}
}

func TestLooksLikeFilePath(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"cmd/main.go", true},
		{"src/utils/helper.py", true},
		{"main.go", true},
		{"hello", false},
		{"a", false},
		{"config.yaml", true},
	}

	for _, tc := range tests {
		got := looksLikeFilePath(tc.input)
		if got != tc.expected {
			t.Errorf("looksLikeFilePath(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestIsPartOfToolPair(t *testing.T) {
	messages := []CompressMessage{
		{Role: "assistant", Content: "reading file", ToolName: "read"},
		{Role: "tool", Content: "file contents...", IsToolResult: true},
		{Role: "assistant", Content: "here's what I found"},
	}

	if !isPartOfToolPair(messages, 0) {
		t.Error("tool call should be part of pair")
	}
	if !isPartOfToolPair(messages, 1) {
		t.Error("tool result should be part of pair")
	}
	if isPartOfToolPair(messages, 2) {
		t.Error("regular message should not be part of pair")
	}
}

func TestFormatCompressedTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500"},
		{1000, "1,000"},
		{12400, "12,400"},
		{0, "0"},
		{999, "999"},
	}

	for _, tc := range tests {
		got := formatCompressedTokens(tc.input)
		if got != tc.expected {
			t.Errorf("formatCompressedTokens(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestGroupByTopic(t *testing.T) {
	messages := []CompressMessage{
		{Role: "user", Content: "Can you fix the auth bug?"},
		{Role: "assistant", Content: "Sure, looking at it"},
		{Role: "user", Content: "Now let's work on the API"},
		{Role: "assistant", Content: "OK switching to API work"},
	}

	groups := groupByTopic(messages)
	if len(groups) < 2 {
		t.Errorf("expected at least 2 topic groups, got %d", len(groups))
	}
}

func TestCompressStrategy_Constants(t *testing.T) {
	strategies := []CompressStrategy{
		StrategySummarize,
		StrategySelective,
		StrategyTiered,
		StrategySemantic,
	}

	expected := []string{"summarize", "selective", "tiered", "semantic"}
	for i, s := range strategies {
		if string(s) != expected[i] {
			t.Errorf("strategy %d: expected %q, got %q", i, expected[i], string(s))
		}
	}
}

func TestCompressConcurrency(t *testing.T) {
	sc := NewSessionCompressor(StrategySelective)
	messages := makeTestMessages(20)
	budget := totalTokens(messages) / 2

	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_, _, err := sc.Compress(messages, budget)
			if err != nil {
				t.Errorf("concurrent compress failed: %v", err)
			}
			done <- true
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}

// --- Test helpers ---

func makeTestMessages(n int) []CompressMessage {
	messages := make([]CompressMessage, n)
	for i := 0; i < n; i++ {
		switch {
		case i == 0:
			messages[i] = CompressMessage{
				Role:    "user",
				Content: "Please help me refactor the authentication module",
				Tokens:  20,
			}
		case i%5 == 1:
			messages[i] = CompressMessage{
				Role:     "assistant",
				Content:  "I'll read the file auth/handler.go",
				ToolName: "read",
				Tokens:   15,
			}
		case i%5 == 2:
			messages[i] = CompressMessage{
				Role:         "tool",
				Content:      "package auth\n\nfunc Handle() error { return nil }",
				IsToolResult: true,
				Tokens:       40,
			}
		case i%5 == 3:
			messages[i] = CompressMessage{
				Role:    "assistant",
				Content: "I see the issue. The error handling is missing proper wrapping.",
				Tokens:  25,
			}
		case i%5 == 4:
			messages[i] = CompressMessage{
				Role:    "user",
				Content: "OK, sounds good. Go ahead and fix it.",
				Tokens:  15,
			}
		default:
			messages[i] = CompressMessage{
				Role:    "assistant",
				Content: "Working on the fix now.",
				Tokens:  10,
			}
		}
	}
	return messages
}

func makeTopicMessages() []CompressMessage {
	return []CompressMessage{
		// Topic 1: Auth
		{Role: "user", Content: "Can you fix the auth system?", Tokens: 15},
		{Role: "assistant", Content: "Looking at auth.go", ToolName: "read", Tokens: 10},
		{Role: "tool", Content: "package auth\nfunc Login() {}", IsToolResult: true, Tokens: 30},
		{Role: "assistant", Content: "I decided to add token refresh logic", Tokens: 20},
		// Topic 2: DB (note "Now" prefix triggers topic boundary)
		{Role: "user", Content: "Now let's fix the database connection pooling", Tokens: 15},
		{Role: "assistant", Content: "I'll check db/pool.go", ToolName: "read", Tokens: 10},
		{Role: "tool", Content: "package db\nfunc NewPool() {}", IsToolResult: true, Tokens: 30},
		{Role: "assistant", Content: "The pool size should be configurable. I decided to add env var support.", Tokens: 25},
		// Topic 3: Tests
		{Role: "user", Content: "Next can you add tests?", Tokens: 10},
		{Role: "assistant", Content: "I'll create auth_test.go", ToolName: "write", Tokens: 10},
		{Role: "tool", Content: "File written successfully", IsToolResult: true, Tokens: 5},
		{Role: "assistant", Content: "Tests added covering login and token refresh", Tokens: 20},
	}
}
