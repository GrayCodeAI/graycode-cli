package engine

import (
	"sync"
	"testing"
)

func TestNewConversationManagerWithConfig(t *testing.T) {
	cfg := ConversationConfig{
		MaxHistory:         100,
		SummarizeThreshold: 50,
		SystemPrompt:       "You are a helpful assistant.",
	}
	cm := NewConversationManager(cfg)

	if cm == nil {
		t.Fatal("expected non-nil ConversationManager")
	}
	if cm.config.MaxHistory != 100 {
		t.Errorf("expected MaxHistory=100, got %d", cm.config.MaxHistory)
	}
	if cm.config.SummarizeThreshold != 50 {
		t.Errorf("expected SummarizeThreshold=50, got %d", cm.config.SummarizeThreshold)
	}

	// System prompt should be added as first message.
	history := cm.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 initial message (system prompt), got %d", len(history))
	}
	if history[0].Role != "system" {
		t.Errorf("expected first message role 'system', got '%s'", history[0].Role)
	}
	if history[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system prompt content: %s", history[0].Content)
	}
}

func TestAddMessageIncrementsHistory(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{})

	before := len(cm.GetHistory())
	cm.AddMessage("user", "hello")
	after := len(cm.GetHistory())

	if after != before+1 {
		t.Errorf("expected history length %d, got %d", before+1, after)
	}

	cm.AddMessage("assistant", "hi there")
	if len(cm.GetHistory()) != 2 {
		t.Errorf("expected 2 messages, got %d", len(cm.GetHistory()))
	}
}

func TestGetHistoryReturnsMessagesInOrder(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{})

	cm.AddMessage("user", "first")
	cm.AddMessage("assistant", "second")
	cm.AddMessage("user", "third")

	history := cm.GetHistory()
	if len(history) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(history))
	}

	expected := []struct {
		role    string
		content string
	}{
		{"user", "first"},
		{"assistant", "second"},
		{"user", "third"},
	}

	for i, exp := range expected {
		if history[i].Role != exp.role {
			t.Errorf("message[%d].Role: expected '%s', got '%s'", i, exp.role, history[i].Role)
		}
		if history[i].Content != exp.content {
			t.Errorf("message[%d].Content: expected '%s', got '%s'", i, exp.content, history[i].Content)
		}
	}
}

func TestGetStateIncludesTokenCountAndSummary(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		SystemPrompt: "system instructions",
	})

	cm.AddMessage("user", "some message here")
	cm.AddMessage("assistant", "a reply")

	state := cm.GetState()

	if state.TokenCount <= 0 {
		t.Errorf("expected positive token count, got %d", state.TokenCount)
	}

	expectedTokens := estimateTokens("system instructions") +
		estimateTokens("some message here") +
		estimateTokens("a reply")
	if state.TokenCount != expectedTokens {
		t.Errorf("expected token count %d, got %d", expectedTokens, state.TokenCount)
	}

	// Summary should be empty when we haven't built one.
	if state.Summary != "" {
		t.Errorf("expected empty summary, got '%s'", state.Summary)
	}

	// Build a summary by adding enough messages.
	for i := 0; i < 15; i++ {
		cm.AddMessage("user", "filler message content")
	}
	cm.BuildSummary()

	state = cm.GetState()
	if state.Summary == "" {
		t.Error("expected non-empty summary after BuildSummary")
	}
}

func TestTrimHistoryKeepsSystemPrompt(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		SystemPrompt: "You are a coding agent.",
	})

	// Add several messages.
	for i := 0; i < 10; i++ {
		cm.AddMessage("user", "msg")
		cm.AddMessage("assistant", "reply")
	}

	cm.TrimHistory(4)

	history := cm.GetHistory()
	if len(history) != 4 {
		t.Fatalf("expected 4 messages after trim, got %d", len(history))
	}

	// First message should still be the system prompt.
	if history[0].Role != "system" {
		t.Errorf("expected first message to be system prompt, got role '%s'", history[0].Role)
	}
	if history[0].Content != "You are a coding agent." {
		t.Errorf("expected system prompt content preserved, got '%s'", history[0].Content)
	}
}

func TestShouldSummarizeReturnsTrueAboveThreshold(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		SummarizeThreshold: 5,
	})

	// Below threshold.
	if cm.ShouldSummarize() {
		t.Error("expected ShouldSummarize=false with 0 messages")
	}

	// Add messages up to threshold.
	for i := 0; i < 5; i++ {
		cm.AddMessage("user", "msg")
	}
	// Exactly at threshold (5 messages, threshold is 5, but condition is >).
	if cm.ShouldSummarize() {
		t.Error("expected ShouldSummarize=false when message count equals threshold")
	}

	cm.AddMessage("user", "one more")
	// Now 6 messages, threshold is 5, so 6 > 5 = true.
	if !cm.ShouldSummarize() {
		t.Error("expected ShouldSummarize=true above threshold")
	}
}

func TestBuildSummaryProducesNonEmptySummary(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		SummarizeThreshold: 3,
	})

	// Add enough messages to have content to summarize.
	for i := 0; i < 20; i++ {
		cm.AddMessage("user", "some conversation content here")
	}

	summary := cm.BuildSummary()
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if len(summary) < 10 {
		t.Errorf("summary too short: '%s'", summary)
	}

	// Verify internal state was updated.
	state := cm.GetState()
	if state.Summary != summary {
		t.Error("expected GetState().Summary to match BuildSummary return value")
	}
}

func TestResetClearsEverything(t *testing.T) {
	cfg := ConversationConfig{
		SystemPrompt:       "system prompt text",
		SummarizeThreshold: 2,
	}
	cm := NewConversationManager(cfg)

	cm.AddMessage("user", "hello")
	cm.AddMessage("assistant", "hi")
	cm.BuildSummary()

	if len(cm.GetHistory()) < 2 {
		t.Fatal("expected messages before reset")
	}

	cm.Reset()

	state := cm.GetState()
	// System prompt should be re-added.
	if len(state.Messages) != 1 {
		t.Fatalf("expected 1 message after reset (system prompt), got %d", len(state.Messages))
	}
	if state.Messages[0].Role != "system" {
		t.Errorf("expected system prompt role after reset, got '%s'", state.Messages[0].Role)
	}
	if state.Summary != "" {
		t.Errorf("expected empty summary after reset, got '%s'", state.Summary)
	}
	expectedTokens := estimateTokens("system prompt text")
	if state.TokenCount != expectedTokens {
		t.Errorf("expected token count %d after reset, got %d", expectedTokens, state.TokenCount)
	}
}

func TestExportMessagesFormat(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{
		SystemPrompt: "be helpful",
	})
	cm.AddMessage("user", "what is 2+2?")
	cm.AddMessage("assistant", "4")

	exported := cm.ExportMessages()
	if len(exported) != 3 {
		t.Fatalf("expected 3 exported messages, got %d", len(exported))
	}

	expected := []struct {
		role    string
		content string
	}{
		{"system", "be helpful"},
		{"user", "what is 2+2?"},
		{"assistant", "4"},
	}

	for i, exp := range expected {
		if exported[i]["role"] != exp.role {
			t.Errorf("exported[%d]['role']: expected '%s', got '%s'", i, exp.role, exported[i]["role"])
		}
		if exported[i]["content"] != exp.content {
			t.Errorf("exported[%d]['content']: expected '%s', got '%s'", i, exp.content, exported[i]["content"])
		}
		// Verify only "role" and "content" keys exist.
		if len(exported[i]) != 2 {
			t.Errorf("exported[%d]: expected exactly 2 keys, got %d", i, len(exported[i]))
		}
	}
}

func TestTokenEstimateRoughAccuracy(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{})

	// estimateTokens uses the BPE-based tok tokenizer.
	content := "this is a test string that should produce some tokens"
	cm.AddMessage("user", content)

	expected := estimateTokens(content)
	got := cm.TokenEstimate()

	if got != expected {
		t.Errorf("expected token estimate %d, got %d", expected, got)
	}

	// The estimate must be non-zero and stay within a sane range for English
	// text (BPE typically lands between ~3 and ~7 chars per token).
	if got <= 0 {
		t.Errorf("token estimate should be positive, got %d", got)
	}
	if cpt := float64(len(content)) / float64(got); cpt < 3 || cpt > 7 {
		t.Errorf("chars-per-token %0.2f outside expected range (3-7) for %d chars and %d tokens", cpt, len(content), got)
	}
}

func TestConcurrentAddMessageSafety(t *testing.T) {
	cm := NewConversationManager(ConversationConfig{})
	const goroutines = 50
	const messagesPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				cm.AddMessage("user", "concurrent message")
			}
		}()
	}

	wg.Wait()

	total := goroutines * messagesPerGoroutine
	if len(cm.GetHistory()) != total {
		t.Errorf("expected %d messages, got %d", total, len(cm.GetHistory()))
	}

	// TokenEstimate should be consistent.
	state := cm.GetState()
	if state.TokenCount <= 0 {
		t.Error("expected positive token count after concurrent writes")
	}
	if state.TokenCount != total*estimateTokens("concurrent message") {
		t.Errorf("expected token count %d, got %d",
			total*estimateTokens("concurrent message"), state.TokenCount)
	}
}
