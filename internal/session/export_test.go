package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleSession() *ExportedSession {
	t0 := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	return &ExportedSession{
		ID:        "abc123",
		Model:     "claude-sonnet-4-6",
		Provider:  "anthropic",
		CreatedAt: t0,
		Messages: []ExportedMessage{
			{Role: "user", Content: "Please implement authentication for the API", Timestamp: t0, TokenCount: 120},
			{Role: "assistant", Content: "I'll implement JWT-based authentication.", Timestamp: t0.Add(5 * time.Second), ToolName: "Edit", ToolResult: "func ValidateToken(token string) error { ... }", TokenCount: 450},
			{Role: "user", Content: "Looks great, now add refresh tokens", Timestamp: t0.Add(2 * time.Minute), TokenCount: 80},
			{Role: "assistant", Content: "Adding refresh token support.", Timestamp: t0.Add(2*time.Minute + 10*time.Second), TokenCount: 380},
		},
		Metadata: map[string]string{"cwd": "/project", "branch": "feature/auth"},
		Stats: SessionExportStats{
			TotalMessages:     4,
			UserMessages:      2,
			AssistantMessages: 2,
			ToolCalls:         1,
			TotalTokens:       1030,
			Duration:          2*time.Minute + 10*time.Second,
		},
	}
}

func TestExportMarkdownFormatting(t *testing.T) {
	session := sampleSession()
	result := ExportMarkdown(session)

	checks := []string{
		"# Session: abc123",
		"Model: claude-sonnet-4-6 | Provider: anthropic",
		"Date: 2024-03-15 10:30 UTC",
		"Duration: 2m10s",
		"Tokens: 1,030",
		"Messages: 4",
		"## User",
		"Please implement authentication for the API",
		"## Assistant",
		"I'll implement JWT-based authentication.",
		"### Tool: Edit",
		"func ValidateToken(token string) error { ... }",
		"---",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("ExportMarkdown missing expected content: %q", check)
		}
	}
}

func TestExportHTMLContainsExpectedElements(t *testing.T) {
	session := sampleSession()
	result := ExportHTML(session)

	checks := []string{
		"<!DOCTYPE html>",
		"<html",
		"Session: abc123",
		"prefers-color-scheme: dark",
		`class="message user"`,
		`class="message assistant"`,
		`class="tool-call"`,
		"<details",
		"<summary>",
		"<pre><code>",
		"</html>",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("ExportHTML missing expected element: %q", check)
		}
	}
}

func TestExportJSONRoundTrip(t *testing.T) {
	session := sampleSession()
	exported := ExportJSON(session)

	imported, err := Import(exported, FormatJSON)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if imported.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", imported.ID, session.ID)
	}
	if imported.Model != session.Model {
		t.Errorf("Model mismatch: got %q, want %q", imported.Model, session.Model)
	}
	if imported.Provider != session.Provider {
		t.Errorf("Provider mismatch: got %q, want %q", imported.Provider, session.Provider)
	}
	if len(imported.Messages) != len(session.Messages) {
		t.Fatalf("Message count mismatch: got %d, want %d", len(imported.Messages), len(session.Messages))
	}
	for i := range session.Messages {
		if imported.Messages[i].Role != session.Messages[i].Role {
			t.Errorf("Message[%d] role mismatch: got %q, want %q", i, imported.Messages[i].Role, session.Messages[i].Role)
		}
		if imported.Messages[i].Content != session.Messages[i].Content {
			t.Errorf("Message[%d] content mismatch", i)
		}
	}
}

func TestExportReplayPreservesTimestamps(t *testing.T) {
	session := sampleSession()
	replayData := ExportReplay(session)

	lines := strings.Split(strings.TrimSpace(replayData), "\n")
	if len(lines) != 4 {
		t.Fatalf("Expected 4 replay lines, got %d", len(lines))
	}

	// Verify first entry has zero delta.
	var first replayEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("Failed to parse first replay entry: %v", err)
	}
	if first.DeltaMs != 0 {
		t.Errorf("First entry delta should be 0, got %d", first.DeltaMs)
	}
	if first.Seq != 1 {
		t.Errorf("First entry seq should be 1, got %d", first.Seq)
	}

	// Verify second entry has a positive delta.
	var second replayEntry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("Failed to parse second replay entry: %v", err)
	}
	if second.DeltaMs != 5000 {
		t.Errorf("Second entry delta should be 5000ms, got %d", second.DeltaMs)
	}

	// Verify timestamps survive round-trip.
	imported, err := Import(replayData, FormatReplay)
	if err != nil {
		t.Fatalf("Import replay failed: %v", err)
	}
	if !imported.Messages[0].Timestamp.Equal(session.Messages[0].Timestamp) {
		t.Errorf("Timestamp not preserved after round-trip")
	}
}

func TestRedactSensitiveRemovesAPIKeys(t *testing.T) {
	session := &ExportedSession{
		ID:       "test",
		Model:    "test-model",
		Provider: "test",
		Messages: []ExportedMessage{
			{Role: "user", Content: "My API key is sk-1234567890abcdefghijklmnop"},
			{Role: "assistant", Content: "I see your key. Also AKIA1234567890123456 is an AWS key."},
			{Role: "user", Content: "bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.something.else"},
			{Role: "assistant", Content: "Set api_key=super_secret_12345678 in config", ToolResult: "password: mysecretpassword123"},
			{Role: "user", Content: "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl is my github token"},
		},
		Metadata: map[string]string{"token": "sk-abcdefghijklmnopqrstuvwxyz"},
	}

	redacted := RedactSensitive(session)

	// Verify secrets are removed from messages.
	for i, msg := range redacted.Messages {
		if strings.Contains(msg.Content, "sk-1234567890") {
			t.Errorf("Message[%d] content still contains sk- key", i)
		}
		if strings.Contains(msg.Content, "AKIA1234567890123456") {
			t.Errorf("Message[%d] content still contains AWS key", i)
		}
		if strings.Contains(msg.Content, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") {
			t.Errorf("Message[%d] content still contains bearer token", i)
		}
		if strings.Contains(msg.Content, "super_secret_12345678") {
			t.Errorf("Message[%d] content still contains API key value", i)
		}
		if strings.Contains(msg.Content, "ghp_ABCDEFGHIJKLMNOP") {
			t.Errorf("Message[%d] content still contains github token", i)
		}
		if strings.Contains(msg.ToolResult, "mysecretpassword123") {
			t.Errorf("Message[%d] tool result still contains password", i)
		}
	}

	// Verify metadata is redacted.
	if strings.Contains(redacted.Metadata["token"], "sk-abcdefghijklmnop") {
		t.Error("Metadata still contains secret token")
	}

	// Verify original is not mutated.
	if !strings.Contains(session.Messages[0].Content, "sk-1234567890") {
		t.Error("Original session was mutated")
	}
}

func TestImportFromJSON(t *testing.T) {
	jsonData := `{
		"id": "imported-session",
		"model": "gpt-4",
		"provider": "openai",
		"created_at": "2024-01-01T00:00:00Z",
		"messages": [
			{"role": "user", "content": "Hello", "timestamp": "2024-01-01T00:00:00Z"},
			{"role": "assistant", "content": "Hi there!", "timestamp": "2024-01-01T00:00:01Z"}
		],
		"stats": {"total_messages": 2, "user_messages": 1, "assistant_messages": 1}
	}`

	session, err := Import(jsonData, FormatJSON)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if session.ID != "imported-session" {
		t.Errorf("ID mismatch: got %q", session.ID)
	}
	if session.Model != "gpt-4" {
		t.Errorf("Model mismatch: got %q", session.Model)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(session.Messages))
	}
	if session.Messages[0].Content != "Hello" {
		t.Errorf("First message content mismatch")
	}
}

func TestStatsCalculation(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	messages := []ExportedMessage{
		{Role: "user", Content: "Hello", Timestamp: t0, TokenCount: 10},
		{Role: "assistant", Content: "Hi", Timestamp: t0.Add(1 * time.Second), ToolName: "Read", TokenCount: 20},
		{Role: "user", Content: "Do X", Timestamp: t0.Add(30 * time.Second), TokenCount: 15},
		{Role: "assistant", Content: "Done", Timestamp: t0.Add(35 * time.Second), ToolName: "Edit", TokenCount: 50},
		{Role: "assistant", Content: "Result", Timestamp: t0.Add(5 * time.Minute), TokenCount: 30},
	}

	stats := CalculateStats(messages)

	if stats.TotalMessages != 5 {
		t.Errorf("TotalMessages: got %d, want 5", stats.TotalMessages)
	}
	if stats.UserMessages != 2 {
		t.Errorf("UserMessages: got %d, want 2", stats.UserMessages)
	}
	if stats.AssistantMessages != 3 {
		t.Errorf("AssistantMessages: got %d, want 3", stats.AssistantMessages)
	}
	if stats.ToolCalls != 2 {
		t.Errorf("ToolCalls: got %d, want 2", stats.ToolCalls)
	}
	if stats.TotalTokens != 125 {
		t.Errorf("TotalTokens: got %d, want 125", stats.TotalTokens)
	}
	if stats.Duration != 5*time.Minute {
		t.Errorf("Duration: got %v, want 5m", stats.Duration)
	}
}

func TestEmptySessionHandling(t *testing.T) {
	empty := &ExportedSession{
		ID:       "empty",
		Model:    "test",
		Provider: "test",
		Messages: []ExportedMessage{},
		Stats:    SessionExportStats{},
	}

	// Markdown should still produce a header.
	md := ExportMarkdown(empty)
	if !strings.Contains(md, "# Session: empty") {
		t.Error("Empty session markdown missing header")
	}

	// JSON should be valid.
	jsonStr := ExportJSON(empty)
	var parsed ExportedSession
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Errorf("Empty session JSON not valid: %v", err)
	}

	// HTML should be valid.
	htmlStr := ExportHTML(empty)
	if !strings.Contains(htmlStr, "<!DOCTYPE html>") {
		t.Error("Empty session HTML missing doctype")
	}

	// Replay should be empty string.
	replayStr := ExportReplay(empty)
	if strings.TrimSpace(replayStr) != "" {
		t.Error("Empty session replay should be empty")
	}

	// Nil session handling.
	if ExportMarkdown(nil) != "" {
		t.Error("Nil session markdown should be empty")
	}
	if ExportHTML(nil) != "" {
		t.Error("Nil session HTML should be empty")
	}
	if ExportJSON(nil) != "{}" {
		t.Error("Nil session JSON should be {}")
	}
	if ExportReplay(nil) != "" {
		t.Error("Nil session replay should be empty")
	}
}

func TestIncludeToolResultsToggle(t *testing.T) {
	session := sampleSession()

	// With tool results included (default).
	exporter := NewSessionExporter()
	resultWith, err := exporter.Export(session, FormatMarkdown)
	if err != nil {
		t.Fatalf("Export with tool results failed: %v", err)
	}
	if !strings.Contains(resultWith, "func ValidateToken") {
		t.Error("Expected tool result to be present when IncludeToolResults=true")
	}

	// Without tool results.
	exporter.IncludeToolResults = false
	resultWithout, err := exporter.Export(session, FormatMarkdown)
	if err != nil {
		t.Fatalf("Export without tool results failed: %v", err)
	}
	if strings.Contains(resultWithout, "func ValidateToken") {
		t.Error("Expected tool result to be absent when IncludeToolResults=false")
	}
}

func TestMaxMessagesTruncation(t *testing.T) {
	session := sampleSession()

	exporter := NewSessionExporter()
	exporter.MaxMessages = 2

	result, err := exporter.Export(session, FormatJSON)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	var exported ExportedSession
	if err := json.Unmarshal([]byte(result), &exported); err != nil {
		t.Fatalf("Failed to parse exported JSON: %v", err)
	}

	if len(exported.Messages) != 2 {
		t.Errorf("Expected 2 messages after truncation, got %d", len(exported.Messages))
	}
	if exported.Messages[0].Role != "user" {
		t.Error("First message should be user after truncation")
	}
}

func TestGenerateShareLinkDeterministic(t *testing.T) {
	session := sampleSession()

	link1 := GenerateShareLink(session)
	link2 := GenerateShareLink(session)

	if link1 != link2 {
		t.Errorf("Share links not deterministic: %q != %q", link1, link2)
	}

	if !strings.HasPrefix(link1, "graycode://share/") {
		t.Errorf("Share link has wrong prefix: %q", link1)
	}

	// Different session should produce different link.
	other := sampleSession()
	other.ID = "different"
	link3 := GenerateShareLink(other)
	if link1 == link3 {
		t.Error("Different sessions should produce different share links")
	}

	// Nil session.
	if GenerateShareLink(nil) != "" {
		t.Error("Nil session share link should be empty")
	}
}

func TestImportFromClaudeFormat(t *testing.T) {
	jsonlData := `{"type":"message","role":"user","content":"Hello Claude","timestamp":"2024-03-15T10:00:00Z","model":"claude-sonnet-4-6"}
{"type":"message","role":"assistant","content":"Hello! How can I help?","timestamp":"2024-03-15T10:00:02Z"}
{"type":"tool_use","role":"assistant","content":"Reading file...","timestamp":"2024-03-15T10:00:05Z","tool_name":"Read","tool_result":"file contents here"}
`

	session, err := ImportFromClaude(jsonlData)
	if err != nil {
		t.Fatalf("ImportFromClaude failed: %v", err)
	}

	if session.Model != "claude-sonnet-4-6" {
		t.Errorf("Model not extracted: got %q", session.Model)
	}
	if session.Provider != "anthropic" {
		t.Errorf("Provider should be anthropic: got %q", session.Provider)
	}
	if len(session.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(session.Messages))
	}
	if session.Messages[2].ToolName != "Read" {
		t.Errorf("Tool name not preserved: got %q", session.Messages[2].ToolName)
	}
	if session.Metadata["source"] != "claude-code" {
		t.Error("Source metadata not set")
	}
}

func TestImportFromAiderFormat(t *testing.T) {
	history := `#### user
Can you fix the bug in main.go?

#### assistant
I'll fix the null pointer issue in main.go.

Here's the fix:
` + "```go" + `
if ptr != nil {
    ptr.Do()
}
` + "```" + `

#### user
Thanks!
`

	session, err := ImportFromAider(history)
	if err != nil {
		t.Fatalf("ImportFromAider failed: %v", err)
	}

	if len(session.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(session.Messages))
	}
	if session.Messages[0].Role != "user" {
		t.Errorf("First message role: got %q, want user", session.Messages[0].Role)
	}
	if !strings.Contains(session.Messages[0].Content, "fix the bug") {
		t.Error("First message content not preserved")
	}
	if session.Messages[1].Role != "assistant" {
		t.Errorf("Second message role: got %q, want assistant", session.Messages[1].Role)
	}
	if !strings.Contains(session.Messages[1].Content, "null pointer") {
		t.Error("Second message content not preserved")
	}
	if session.Metadata["source"] != "aider" {
		t.Error("Source metadata not set")
	}
}

func TestExporterRedactSecrets(t *testing.T) {
	session := &ExportedSession{
		ID:       "secret-test",
		Model:    "test",
		Provider: "test",
		Messages: []ExportedMessage{
			{Role: "user", Content: "Use api_key=sk-supersecretkey1234567890abcdef"},
		},
	}

	exporter := NewSessionExporter()
	exporter.RedactSecrets = true

	result, err := exporter.Export(session, FormatMarkdown)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if strings.Contains(result, "sk-supersecretkey1234567890abcdef") {
		t.Error("Secret not redacted when RedactSecrets=true")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("Expected [REDACTED] marker in output")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{5 * time.Minute, "5m"},
		{65 * time.Minute, "1h5m"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.input)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{15420, "15,420"},
		{1000000, "1,000,000"},
	}

	for _, tt := range tests {
		result := formatNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestImportUnsupportedFormat(t *testing.T) {
	_, err := Import("data", FormatMarkdown)
	if err == nil {
		t.Error("Expected error for unsupported import format")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestExportUnsupportedFormat(t *testing.T) {
	exporter := NewSessionExporter()
	_, err := exporter.Export(sampleSession(), ExportFormat("unknown"))
	if err == nil {
		t.Error("Expected error for unsupported export format")
	}
}

func TestExportNilSession(t *testing.T) {
	exporter := NewSessionExporter()
	_, err := exporter.Export(nil, FormatJSON)
	if err == nil {
		t.Error("Expected error for nil session")
	}
}

func TestImportFromCursorFormat(t *testing.T) {
	cursorJSON := `{
		"conversationId": "cursor-sess-1",
		"model": "gpt-4o",
		"messages": [
			{"speaker": "human", "text": "How do I implement JWT auth in Go?", "timestamp": 1700000000000},
			{"speaker": "ai", "text": "Here is how you use golang-jwt to verify tokens...", "timestamp": 1700000001000}
		]
	}`

	sess, err := Import(cursorJSON, FormatCursor)
	if err != nil {
		t.Fatalf("Import(FormatCursor) failed: %v", err)
	}

	if sess.ID != "cursor-sess-1" {
		t.Errorf("got ID %q, want cursor-sess-1", sess.ID)
	}
	if sess.Provider != "cursor" {
		t.Errorf("got provider %q, want cursor", sess.Provider)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(sess.Messages))
	}
	if sess.Messages[0].Role != "user" || !strings.Contains(sess.Messages[0].Content, "JWT auth") {
		t.Errorf("message[0] = %+v", sess.Messages[0])
	}
	if sess.Messages[1].Role != "assistant" || !strings.Contains(sess.Messages[1].Content, "golang-jwt") {
		t.Errorf("message[1] = %+v", sess.Messages[1])
	}
}

func TestImportFromOpenAIFormat(t *testing.T) {
	openAIJSON := `{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Explain goroutines in 1 sentence."},
			{"role": "assistant", "content": "Goroutines are lightweight threads managed by the Go runtime."}
		]
	}`

	sess, err := Import(openAIJSON, FormatOpenAI)
	if err != nil {
		t.Fatalf("Import(FormatOpenAI) failed: %v", err)
	}

	if sess.Model != "gpt-4o-mini" {
		t.Errorf("got Model %q, want gpt-4o-mini", sess.Model)
	}
	if sess.Provider != "openai" {
		t.Errorf("got Provider %q, want openai", sess.Provider)
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(sess.Messages))
	}
	if sess.Messages[1].Role != "user" || sess.Messages[1].Content != "Explain goroutines in 1 sentence." {
		t.Errorf("message[1] = %+v", sess.Messages[1])
	}

	// Test Export to OpenAI format
	exporter := NewSessionExporter()
	out, err := exporter.Export(sess, FormatOpenAI)
	if err != nil {
		t.Fatalf("Export(FormatOpenAI) failed: %v", err)
	}
	if !strings.Contains(out, "gpt-4o-mini") || !strings.Contains(out, "lightweight threads") {
		t.Errorf("Export(FormatOpenAI) output = %s", out)
	}
}
