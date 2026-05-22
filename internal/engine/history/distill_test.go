package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistillNewPipeline(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")

	if dp.Dir != "/tmp/distill-test" {
		t.Errorf("expected dir /tmp/distill-test, got %s", dp.Dir)
	}
	if dp.MinQuality != 0.7 {
		t.Errorf("expected min quality 0.7, got %f", dp.MinQuality)
	}
	if dp.TargetModel != "claude-haiku" {
		t.Errorf("expected target model claude-haiku, got %s", dp.TargetModel)
	}
	if len(dp.Examples) != 0 {
		t.Errorf("expected 0 examples, got %d", len(dp.Examples))
	}
}

func TestDistillCapture(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")

	// Below min quality - should not be captured
	dp.Capture("sys", "user", "assistant", nil, 0.5, "gpt-4")
	if len(dp.Examples) != 0 {
		t.Errorf("expected 0 examples for low quality, got %d", len(dp.Examples))
	}

	// Above min quality - should be captured
	dp.Capture("You are helpful.", "Write a function", "Here is the function...", []string{"code_edit"}, 0.9, "claude-sonnet-4-6")
	if len(dp.Examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(dp.Examples))
	}

	ex := dp.Examples[0]
	if ex.SystemPrompt != "You are helpful." {
		t.Errorf("unexpected system prompt: %s", ex.SystemPrompt)
	}
	if ex.UserMessage != "Write a function" {
		t.Errorf("unexpected user message: %s", ex.UserMessage)
	}
	if ex.AssistantResponse != "Here is the function..." {
		t.Errorf("unexpected assistant response: %s", ex.AssistantResponse)
	}
	if ex.Quality != 0.9 {
		t.Errorf("expected quality 0.9, got %f", ex.Quality)
	}
	if ex.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", ex.Model)
	}
	if len(ex.ToolCalls) != 1 || ex.ToolCalls[0] != "code_edit" {
		t.Errorf("unexpected tool calls: %v", ex.ToolCalls)
	}
	if ex.ID == "" {
		t.Error("expected non-empty ID")
	}
	if ex.Tokens == 0 {
		t.Error("expected non-zero tokens")
	}
	if dp.SourceModel != "claude-sonnet-4-6" {
		t.Errorf("expected source model to be set, got %s", dp.SourceModel)
	}
}

func TestDistillCaptureCustomMinQuality(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.9

	dp.Capture("sys", "user", "assistant", nil, 0.85, "gpt-4")
	if len(dp.Examples) != 0 {
		t.Errorf("expected 0 examples with min quality 0.9, got %d", len(dp.Examples))
	}

	dp.Capture("sys", "user", "assistant", nil, 0.95, "gpt-4")
	if len(dp.Examples) != 1 {
		t.Errorf("expected 1 example, got %d", len(dp.Examples))
	}
}

func TestDistillExportJSONL(t *testing.T) {
	dir := t.TempDir()
	dp := NewDistillationPipeline(dir)

	dp.Capture("System prompt", "Hello user", "Hello! How can I help?", nil, 0.85, "gpt-4o")
	dp.Capture("System prompt", "Write code", "Here is some code implementing func main", nil, 0.92, "claude-sonnet-4-6")

	outPath := filepath.Join(dir, "output", "train.jsonl")
	if err := dp.ExportJSONL(outPath); err != nil {
		t.Fatalf("ExportJSONL failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var record map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("unmarshal line failed: %v", err)
	}

	messages, ok := record["messages"].([]interface{})
	if !ok {
		t.Fatal("expected messages array")
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	msg0 := messages[0].(map[string]interface{})
	if msg0["role"] != "system" {
		t.Errorf("expected first message role to be system, got %s", msg0["role"])
	}
}

func TestDistillExportOpenAI(t *testing.T) {
	dir := t.TempDir()
	dp := NewDistillationPipeline(dir)

	dp.Capture("sys", "user msg", "assistant msg", nil, 0.8, "gpt-4")

	outPath := filepath.Join(dir, "openai.jsonl")
	if err := dp.ExportOpenAI(outPath); err != nil {
		t.Fatalf("ExportOpenAI failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}

	var record map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	messages := record["messages"].([]interface{})
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}
}

func TestDistillExportAnthropicFormat(t *testing.T) {
	dir := t.TempDir()
	dp := NewDistillationPipeline(dir)

	dp.Capture("system prompt here", "user question", "assistant answer", nil, 0.88, "claude-sonnet-4-6")

	outPath := filepath.Join(dir, "anthropic.jsonl")
	if err := dp.ExportAnthropicFormat(outPath); err != nil {
		t.Fatalf("ExportAnthropicFormat failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}

	var record map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if record["system"] != "system prompt here" {
		t.Errorf("expected system field, got %v", record["system"])
	}

	messages := record["messages"].([]interface{})
	if len(messages) != 2 {
		t.Errorf("expected 2 messages (no system in messages), got %d", len(messages))
	}

	msg0 := messages[0].(map[string]interface{})
	if msg0["role"] != "user" {
		t.Errorf("expected first message role to be user, got %s", msg0["role"])
	}
	msg1 := messages[1].(map[string]interface{})
	if msg1["role"] != "assistant" {
		t.Errorf("expected second message role to be assistant, got %s", msg1["role"])
	}
}

func TestDistillFilter(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.5

	dp.Capture("sys", "Write code implementing a function", "func main() {}", nil, 0.6, "gpt-4")
	dp.Capture("sys", "Review this code and suggest improvements", "Looks good, but refactor X", nil, 0.8, "gpt-4")
	dp.Capture("sys", "Fix this error in debug mode", "The bug is on line 5", nil, 0.95, "claude-sonnet-4-6")

	// Filter by quality only
	results := dp.Filter(0.75, nil)
	if len(results) != 2 {
		t.Errorf("expected 2 results with quality >= 0.75, got %d", len(results))
	}

	// Filter by quality and tags
	results = dp.Filter(0.5, []string{"coding"})
	if len(results) < 1 {
		t.Errorf("expected at least 1 result with coding tag, got %d", len(results))
	}

	// Filter with no matching tags
	results = dp.Filter(0.5, []string{"nonexistent"})
	if len(results) != 0 {
		t.Errorf("expected 0 results with nonexistent tag, got %d", len(results))
	}
}

func TestDistillDeduplicate(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.5

	// Add near-duplicate examples
	dp.Capture("sys", "Write a hello world function in Go", "Here is the hello world function", nil, 0.8, "gpt-4")
	dp.Capture("sys", "Write a hello world function in Go", "Here is the hello world function", nil, 0.85, "gpt-4")
	// Add a distinct example
	dp.Capture("sys", "Explain quantum computing concepts", "Quantum computing uses qubits...", nil, 0.9, "claude-sonnet-4-6")

	if len(dp.Examples) != 3 {
		t.Fatalf("expected 3 examples before dedup, got %d", len(dp.Examples))
	}

	dp.Deduplicate()

	if len(dp.Examples) != 2 {
		t.Errorf("expected 2 examples after dedup, got %d", len(dp.Examples))
	}
}

func TestDistillDeduplicateEmpty(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.Deduplicate() // Should not panic
	if len(dp.Examples) != 0 {
		t.Errorf("expected 0 examples, got %d", len(dp.Examples))
	}
}

func TestDistillStats(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.5

	dp.Capture("sys", "Write a function to implement sorting", "func sort() {...}", nil, 0.8, "gpt-4o")
	dp.Capture("sys", "Review this code for bugs", "I found a bug", nil, 0.9, "claude-sonnet-4-6")
	dp.Capture("sys", "Fix the error in this code", "Fixed", nil, 0.7, "gpt-4o")

	stats := dp.Stats()

	if stats.TotalExamples != 3 {
		t.Errorf("expected 3 total examples, got %d", stats.TotalExamples)
	}
	if stats.AvgQuality < 0.79 || stats.AvgQuality > 0.81 {
		t.Errorf("expected avg quality ~0.8, got %f", stats.AvgQuality)
	}
	if stats.ByModel["gpt-4o"] != 2 {
		t.Errorf("expected 2 gpt-4o examples, got %d", stats.ByModel["gpt-4o"])
	}
	if stats.ByModel["claude-sonnet-4-6"] != 1 {
		t.Errorf("expected 1 claude-sonnet-4-6 example, got %d", stats.ByModel["claude-sonnet-4-6"])
	}
	if stats.TotalTokens == 0 {
		t.Error("expected non-zero total tokens")
	}
	if stats.EstimatedCost <= 0 {
		t.Error("expected positive estimated cost")
	}
}

func TestDistillStatsEmpty(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	stats := dp.Stats()

	if stats.TotalExamples != 0 {
		t.Errorf("expected 0 examples, got %d", stats.TotalExamples)
	}
	if stats.AvgQuality != 0 {
		t.Errorf("expected 0 avg quality, got %f", stats.AvgQuality)
	}
}

func TestDistillFormatStats(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.5

	dp.Capture("sys", "Implement a sorting function in code", "func sort() {...}", nil, 0.85, "claude-sonnet-4-6")
	dp.Capture("sys", "Review this code for improvements", "Consider refactoring", nil, 0.9, "claude-sonnet-4-6")

	output := dp.FormatStats()

	if !strings.Contains(output, "Distillation Pipeline:") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "Examples: 2") {
		t.Errorf("expected examples count in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Avg quality:") {
		t.Error("expected avg quality in output")
	}
	if !strings.Contains(output, "claude-sonnet-4-6") {
		t.Error("expected model name in output")
	}
	if !strings.Contains(output, "claude-haiku fine-tuning") {
		t.Errorf("expected target model in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Tokens:") {
		t.Error("expected tokens in output")
	}
}

func TestDistillSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	dp := NewDistillationPipeline(dir)
	dp.TargetModel = "claude-haiku"
	dp.SourceModel = "claude-sonnet-4-6"

	dp.Capture("sys", "Write code for a web server", "package main\nimport net/http", []string{"code_edit"}, 0.88, "claude-sonnet-4-6")
	dp.Capture("sys", "Fix this bug in the handler", "Fixed the nil pointer", nil, 0.92, "claude-sonnet-4-6")

	if err := dp.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "distill_pipeline.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected pipeline file to exist")
	}

	// Load into new pipeline
	dp2 := NewDistillationPipeline(dir)
	if err := dp2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(dp2.Examples) != 2 {
		t.Errorf("expected 2 examples after load, got %d", len(dp2.Examples))
	}
	if dp2.TargetModel != "claude-haiku" {
		t.Errorf("expected target model claude-haiku, got %s", dp2.TargetModel)
	}
	if dp2.SourceModel != "claude-sonnet-4-6" {
		t.Errorf("expected source model claude-sonnet-4-6, got %s", dp2.SourceModel)
	}
	if dp2.Examples[0].SystemPrompt != "sys" {
		t.Errorf("expected system prompt 'sys', got %s", dp2.Examples[0].SystemPrompt)
	}
}

func TestDistillLoadNonexistent(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/nonexistent-distill-dir-12345")
	err := dp.Load()
	if err == nil {
		t.Error("expected error loading from nonexistent dir")
	}
}

func TestDistillPrune(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.5

	dp.Capture("sys", "task1 with some code content", "response1", nil, 0.6, "gpt-4")
	dp.Capture("sys", "task2 is about reviewing code", "response2", nil, 0.9, "gpt-4")
	dp.Capture("sys", "task3 involves debugging errors", "response3", nil, 0.75, "gpt-4")
	dp.Capture("sys", "task4 about deployment config", "response4", nil, 0.85, "gpt-4")

	dp.Prune(2)

	if len(dp.Examples) != 2 {
		t.Fatalf("expected 2 examples after prune, got %d", len(dp.Examples))
	}

	// Should keep the highest quality ones
	if dp.Examples[0].Quality != 0.9 {
		t.Errorf("expected first example quality 0.9, got %f", dp.Examples[0].Quality)
	}
	if dp.Examples[1].Quality != 0.85 {
		t.Errorf("expected second example quality 0.85, got %f", dp.Examples[1].Quality)
	}
}

func TestDistillPruneNoOp(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")
	dp.MinQuality = 0.5

	dp.Capture("sys", "task1 with coding content", "response1", nil, 0.8, "gpt-4")

	dp.Prune(10) // maxExamples > current count

	if len(dp.Examples) != 1 {
		t.Errorf("expected 1 example (no prune needed), got %d", len(dp.Examples))
	}
}

func TestDistillInferTags(t *testing.T) {
	tags := inferTags("implement a new function", "here is the code")
	found := false
	for _, tag := range tags {
		if tag == "coding" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'coding' tag, got %v", tags)
	}

	tags = inferTags("review this pull request", "looks good, suggest improvements")
	found = false
	for _, tag := range tags {
		if tag == "review" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'review' tag, got %v", tags)
	}

	tags = inferTags("hello", "hi")
	found = false
	for _, tag := range tags {
		if tag == "general" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'general' tag for generic content, got %v", tags)
	}
}

func TestDistillSimilarity(t *testing.T) {
	// Identical strings
	s := similarity("hello world foo bar", "hello world foo bar")
	if s != 1.0 {
		t.Errorf("expected similarity 1.0 for identical strings, got %f", s)
	}

	// Completely different strings
	s = similarity("abcdefghijk", "zyxwvutsrqp")
	if s > 0.2 {
		t.Errorf("expected low similarity for different strings, got %f", s)
	}

	// Similar strings
	s = similarity("hello world this is a test", "hello world this is a test!")
	if s < 0.8 {
		t.Errorf("expected high similarity for near-identical strings, got %f", s)
	}

	// Empty strings
	s = similarity("", "hello")
	if s != 0.0 {
		t.Errorf("expected 0.0 for empty string, got %f", s)
	}
}

func TestDistillGenerateID(t *testing.T) {
	id1 := generateDistillID("sys", "user", "assistant")
	id2 := generateDistillID("sys", "user", "assistant")
	id3 := generateDistillID("sys", "different", "assistant")

	if id1 != id2 {
		t.Error("same inputs should produce same ID")
	}
	if id1 == id3 {
		t.Error("different inputs should produce different IDs")
	}
	if !strings.HasPrefix(id1, "distill_") {
		t.Errorf("expected 'distill_' prefix, got %s", id1)
	}
}

func TestDistillEstimateTokens(t *testing.T) {
	tokens := estimateDistillTokens("hello world") // 11 chars -> ~2-3 tokens
	if tokens == 0 {
		t.Error("expected non-zero token estimate")
	}

	tokens = estimateDistillTokens("")
	if tokens != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", tokens)
	}
}

func TestDistillFormatTokenCount(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{500, "500"},
		{5000, "5K"},
		{234000, "234K"},
		{1500000, "1.5M"},
	}

	for _, tt := range tests {
		result := formatDistillTokenCount(tt.tokens)
		if result != tt.expected {
			t.Errorf("formatDistillTokenCount(%d) = %s, expected %s", tt.tokens, result, tt.expected)
		}
	}
}

func TestDistillConcurrentCapture(t *testing.T) {
	dp := NewDistillationPipeline("/tmp/distill-test")

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			dp.Capture("sys", "user message for concurrency test", "response", nil, 0.8+float64(n)*0.01, "gpt-4")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(dp.Examples) != 10 {
		t.Errorf("expected 10 examples from concurrent capture, got %d", len(dp.Examples))
	}
}

func TestDistillExportJSONLNoSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	dp := NewDistillationPipeline(dir)

	dp.Capture("", "user question", "answer", nil, 0.8, "gpt-4")

	outPath := filepath.Join(dir, "no_sys.jsonl")
	if err := dp.ExportJSONL(outPath); err != nil {
		t.Fatalf("ExportJSONL failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}

	var record map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	messages := record["messages"].([]interface{})
	// Without system prompt, should only have user and assistant
	if len(messages) != 2 {
		t.Errorf("expected 2 messages without system prompt, got %d", len(messages))
	}
	msg0 := messages[0].(map[string]interface{})
	if msg0["role"] != "user" {
		t.Errorf("expected first message role to be user, got %s", msg0["role"])
	}
}
