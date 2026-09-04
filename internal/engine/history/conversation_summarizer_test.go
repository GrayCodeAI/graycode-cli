package history

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
)

func TestNewConversationSummarizer(t *testing.T) {
	cs := NewConversationSummarizer()
	if cs == nil {
		t.Fatal("NewConversationSummarizer returned nil")
	}
}

func TestOneLine_Empty(t *testing.T) {
	cs := NewConversationSummarizer()
	result := cs.OneLine(nil)
	if result != "Empty conversation" {
		t.Errorf("expected 'Empty conversation', got %q", result)
	}
}

func TestOneLine_WithContent(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Please implement JWT authentication for the API"},
		{Role: "assistant", Content: "I'll implement JWT auth. Let me create the auth middleware."},
		{Role: "assistant", Content: "Created auth/jwt.go with token generation and validation.", ToolName: "write_file"},
		{Role: "assistant", Content: "Added tests in auth/jwt_test.go", ToolName: "write_file"},
		{Role: "user", Content: "There's a bug in the token expiry"},
		{Role: "assistant", Content: "Fixed the expiry calculation in auth/jwt.go", ToolName: "edit_file"},
	}

	result := cs.OneLine(messages)
	if result == "" {
		t.Fatal("OneLine returned empty string")
	}
	// Should mention key topics
	if !strings.Contains(strings.ToLower(result), "auth") && !strings.Contains(strings.ToLower(result), "jwt") {
		t.Errorf("expected mention of auth/jwt in one-line summary, got %q", result)
	}
}

func TestParagraph_Empty(t *testing.T) {
	cs := NewConversationSummarizer()
	result := cs.Paragraph(nil)
	if result != "No messages to summarize." {
		t.Errorf("expected 'No messages to summarize.', got %q", result)
	}
}

func TestParagraph_WithContent(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Set up the database migration for the users table"},
		{Role: "assistant", Content: "I'll create the migration. Using SQL for the schema."},
		{Role: "assistant", Content: "Created migrations/001_users.sql", ToolName: "write_file"},
		{Role: "user", Content: "Also add an index on email"},
		{Role: "assistant", Content: "Added the index in migrations/001_users.sql", ToolName: "edit_file"},
	}

	result := cs.Paragraph(messages)
	sentences := strings.Split(result, ". ")
	if len(sentences) < 2 {
		t.Errorf("expected at least 2 sentences in paragraph, got %d: %q", len(sentences), result)
	}
}

func TestDetailed_WithContent(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Implement the caching layer for our API"},
		{Role: "assistant", Content: "I'll add Redis-based caching. Let me create the cache middleware."},
		{Role: "assistant", Content: "Created cache/redis.go", ToolName: "write_file"},
		{Role: "assistant", Content: "Error: connection refused", IsError: true},
		{Role: "user", Content: "Use in-memory cache instead of Redis"},
		{Role: "assistant", Content: "Switching to an in-memory cache. Let's use a simple map with TTL."},
		{Role: "assistant", Content: "Created cache/memory.go", ToolName: "write_file"},
		{Role: "assistant", Content: "Added tests in cache/memory_test.go", ToolName: "write_file"},
	}

	result := cs.Detailed(messages)
	if !strings.Contains(result, "## Overview") {
		t.Error("detailed summary should contain '## Overview' section")
	}
	if !strings.Contains(result, "## Files Discussed") {
		t.Error("detailed summary should contain '## Files Discussed' section")
	}
	if !strings.Contains(result, "## Tools Used") {
		t.Error("detailed summary should contain '## Tools Used' section")
	}
	if !strings.Contains(result, "## Errors") {
		t.Error("detailed summary should contain '## Errors' section")
	}
}

func TestStructured_AllFieldsPopulated(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Add JWT authentication to the API endpoints"},
		{Role: "assistant", Content: "I'll implement JWT auth. Let's use jwt-go for token handling."},
		{Role: "assistant", Content: "Created auth/middleware.go", ToolName: "write_file"},
		{Role: "assistant", Content: "Created auth/jwt.go", ToolName: "write_file"},
		{Role: "assistant", Content: "Running tests", ToolName: "run_command"},
		{Role: "assistant", Content: "Test failed: missing config", IsError: true},
		{Role: "assistant", Content: "Fixed config loading in config/auth.yaml", ToolName: "edit_file"},
		{Role: "user", Content: "Use RS256 instead of HS256 for signing"},
		{Role: "assistant", Content: "Switching to RS256. Updated auth/jwt.go", ToolName: "edit_file"},
	}

	summary := cs.Structured(messages)
	if summary == nil {
		t.Fatal("Structured returned nil")
	}
	if summary.Level != string(SummaryStructured) {
		t.Errorf("expected level %q, got %q", SummaryStructured, summary.Level)
	}
	if summary.Content == "" {
		t.Error("Content should not be empty")
	}
	if len(summary.Topics) == 0 {
		t.Error("Topics should not be empty")
	}
	if len(summary.FilesDiscussed) == 0 {
		t.Error("FilesDiscussed should not be empty")
	}
	if len(summary.ToolsUsed) == 0 {
		t.Error("ToolsUsed should not be empty")
	}
	if summary.TokensSaved <= 0 {
		t.Error("TokensSaved should be positive")
	}
}

func TestSummarize_Levels(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Fix the bug in the auth module"},
		{Role: "assistant", Content: "Looking at auth/handler.go for the bug."},
		{Role: "assistant", Content: "Fixed the nil pointer in auth/handler.go", ToolName: "edit_file"},
	}

	tests := []struct {
		level   SummaryLevel
		checkFn func(*Summary) bool
		errMsg  string
	}{
		{SummaryOneLine, func(s *Summary) bool { return s.Level == "one_line" && s.Content != "" }, "one_line should have content"},
		{SummaryParagraph, func(s *Summary) bool { return s.Level == "paragraph" && len(s.Topics) > 0 }, "paragraph should have topics"},
		{SummaryDetailed, func(s *Summary) bool { return s.Level == "detailed" && len(s.FilesDiscussed) > 0 }, "detailed should have files"},
		{SummaryStructured, func(s *Summary) bool { return s.Level == "structured" && s.ToolsUsed != nil }, "structured should have tools"},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			result := cs.Summarize(messages, tt.level)
			if result == nil {
				t.Fatal("Summarize returned nil")
			}
			if !tt.checkFn(result) {
				t.Error(tt.errMsg)
			}
		})
	}
}

func TestExtractTopics(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "We need to add authentication and write tests for it"},
		{Role: "assistant", Content: "I'll implement the auth module with JWT tokens and add comprehensive tests."},
		{Role: "user", Content: "Also configure the CI pipeline to run tests automatically"},
	}

	topics := cs.ExtractTopics(messages)
	if len(topics) == 0 {
		t.Fatal("expected at least one topic")
	}

	// Should detect testing and auth topics
	found := make(map[string]bool)
	for _, t := range topics {
		found[t] = true
	}
	if !found["testing"] {
		t.Errorf("expected 'testing' topic, got topics: %v", topics)
	}
}

func TestExtractDecisions(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "How should we handle sessions?"},
		{Role: "assistant", Content: "Let's use JWT instead of server-side sessions for better scalability."},
		{Role: "user", Content: "Good idea. What about rate limiting?"},
		{Role: "assistant", Content: "I'll add rate limiting using a token bucket algorithm."},
	}

	decisions := cs.ExtractDecisions(messages)
	if len(decisions) == 0 {
		t.Fatal("expected at least one decision")
	}
}

func TestExtractFilesDiscussed(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "assistant", Content: "I'll modify main.go and add auth/handler.go"},
		{Role: "assistant", Content: "Updated config.yaml with the new settings"},
		{Role: "assistant", Content: "Tests pass in auth/handler_test.go"},
	}

	files := cs.ExtractFilesDiscussed(messages)
	if len(files) == 0 {
		t.Fatal("expected at least one file")
	}

	found := make(map[string]bool)
	for _, f := range files {
		found[f] = true
	}
	if !found["main.go"] {
		t.Errorf("expected main.go in files, got: %v", files)
	}
	if !found["config.yaml"] {
		t.Errorf("expected config.yaml in files, got: %v", files)
	}
}

func TestGenerateTitle_Empty(t *testing.T) {
	cs := NewConversationSummarizer()
	title := cs.GenerateTitle(nil)
	if title != "Empty Session" {
		t.Errorf("expected 'Empty Session', got %q", title)
	}
}

func TestGenerateTitle_WithTopics(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Implement JWT authentication for our API"},
		{Role: "assistant", Content: "I'll create the JWT auth middleware and tests."},
		{Role: "assistant", Content: "Created auth/jwt.go", ToolName: "write_file"},
		{Role: "user", Content: "There's a bug with token refresh"},
		{Role: "assistant", Content: "Fixed the token refresh bug in auth/jwt.go", ToolName: "edit_file"},
	}

	title := cs.GenerateTitle(messages)
	if title == "" {
		t.Fatal("GenerateTitle returned empty string")
	}
	if len(title) > 70 {
		t.Errorf("title exceeds 70 chars: %q (len=%d)", title, len(title))
	}
}

func TestGenerateTitle_FallbackToFirstMessage(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "What time is it in Tokyo?"},
		{Role: "assistant", Content: "It's currently 3:00 PM in Tokyo."},
	}

	title := cs.GenerateTitle(messages)
	if title == "" || title == "Empty Session" {
		t.Errorf("expected a meaningful title, got %q", title)
	}
}

func TestConversationSummarizerFormatSummary(t *testing.T) {
	cs := NewConversationSummarizer()
	summary := &Summary{
		Level:          "structured",
		Content:        "Implemented auth with JWT",
		Topics:         []string{"authentication", "testing"},
		Decisions:      []string{"Use JWT over sessions"},
		FilesDiscussed: []string{"auth/jwt.go", "auth/jwt_test.go"},
		ToolsUsed:      map[string]int{"write_file": 3, "run_command": 1},
		TokensSaved:    1500,
	}

	result := cs.FormatSummary(summary)
	if !strings.Contains(result, "structured") {
		t.Error("formatted summary should contain level")
	}
	if !strings.Contains(result, "authentication") {
		t.Error("formatted summary should contain topics")
	}
	if !strings.Contains(result, "auth/jwt.go") {
		t.Error("formatted summary should contain files")
	}
	if !strings.Contains(result, "1500") {
		t.Error("formatted summary should contain tokens saved")
	}
}

func TestFormatSummary_Nil(t *testing.T) {
	cs := NewConversationSummarizer()
	result := cs.FormatSummary(nil)
	if result != "" {
		t.Errorf("expected empty string for nil summary, got %q", result)
	}
}

func TestCompareMessages_BothEmpty(t *testing.T) {
	cs := NewConversationSummarizer()
	result := cs.CompareMessages(nil, nil)
	if result != "No messages in either state." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestCompareMessages_NewConversation(t *testing.T) {
	cs := NewConversationSummarizer()
	after := []SumMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}
	result := cs.CompareMessages(nil, after)
	if !strings.Contains(result, "2 messages") {
		t.Errorf("expected mention of 2 messages, got %q", result)
	}
}

func TestCompareMessages_Cleared(t *testing.T) {
	cs := NewConversationSummarizer()
	before := []SumMessage{
		{Role: "user", Content: "Hello"},
	}
	result := cs.CompareMessages(before, nil)
	if result != "Conversation was cleared." {
		t.Errorf("expected 'Conversation was cleared.', got %q", result)
	}
}

func TestCompareMessages_Changes(t *testing.T) {
	cs := NewConversationSummarizer()
	before := []SumMessage{
		{Role: "user", Content: "Fix the auth bug"},
		{Role: "assistant", Content: "Looking at auth/handler.go"},
	}
	after := []SumMessage{
		{Role: "user", Content: "Fix the auth bug"},
		{Role: "assistant", Content: "Looking at auth/handler.go"},
		{Role: "assistant", Content: "Fixed the bug. Also added tests in auth/handler_test.go"},
		{Role: "user", Content: "Now deploy it with the CI pipeline"},
	}

	result := cs.CompareMessages(before, after)
	if !strings.Contains(result, "new message") {
		t.Errorf("expected mention of new messages, got %q", result)
	}
}

func TestExtractTopics_NoTopics(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	topics := cs.ExtractTopics(messages)
	if len(topics) != 0 {
		t.Errorf("expected no topics, got %v", topics)
	}
}

func TestIsLikelyFile(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"main.go", true},
		{"auth/handler.go", true},
		{"config.yaml", true},
		{"Dockerfile", false}, // no extension
		{"hello", false},
		{"test.py", true},
		{"styles.css", true},
		{"e.g", false}, // not a valid ext
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isLikelyFile(tt.input)
			if result != tt.expected {
				t.Errorf("isLikelyFile(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPluralS(t *testing.T) {
	if pluralS(1) != "" {
		t.Error("pluralS(1) should be empty")
	}
	if pluralS(0) != "s" {
		t.Error("pluralS(0) should be 's'")
	}
	if pluralS(5) != "s" {
		t.Error("pluralS(5) should be 's'")
	}
}

func TestConversationSummarizerEstimateTokens(t *testing.T) {
	result := token.CountTokens("hello world test string")
	if result <= 0 {
		t.Errorf("expected positive token estimate, got %d", result)
	}
}

func TestConcurrency(t *testing.T) {
	cs := NewConversationSummarizer()
	messages := []SumMessage{
		{Role: "user", Content: "Implement the auth system with JWT tokens"},
		{Role: "assistant", Content: "Creating auth/jwt.go with token validation", ToolName: "write_file"},
		{Role: "assistant", Content: "Added tests", ToolName: "write_file"},
	}

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = cs.OneLine(messages)
			_ = cs.Paragraph(messages)
			_ = cs.Detailed(messages)
			_ = cs.Structured(messages)
			_ = cs.GenerateTitle(messages)
			_ = cs.ExtractTopics(messages)
			_ = cs.ExtractDecisions(messages)
			_ = cs.ExtractFilesDiscussed(messages)
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
