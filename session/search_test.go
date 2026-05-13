package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSearchEngine(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")
	if se == nil {
		t.Fatal("expected non-nil SearchEngine")
	}
	if se.SessionDir != "/tmp/sessions" {
		t.Errorf("expected SessionDir=/tmp/sessions, got %s", se.SessionDir)
	}
	if se.Index == nil {
		t.Fatal("expected non-nil Index map")
	}
}

func TestIndexAndSearch(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	messages := []Message{
		{Role: "user", Content: "implement the auth middleware with JWT validation"},
		{Role: "assistant", Content: "I'll create the authentication middleware using JSON Web Tokens"},
		{Role: "user", Content: "also add rate limiting to the API endpoints"},
	}

	if err := se.IndexSession("session-abc123", messages); err != nil {
		t.Fatalf("IndexSession failed: %v", err)
	}

	results := se.Search("auth middleware", SearchOptions{MaxResults: 10})
	if len(results) == 0 {
		t.Fatal("expected search results for 'auth middleware'")
	}

	// First result should be the message about auth middleware
	found := false
	for _, r := range results {
		if r.SessionID == "session-abc123" && strings.Contains(r.Content, "auth") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find message containing 'auth'")
	}
}

func TestBM25ScoringRanksRelevantHigher(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	// Session with highly relevant content
	messages1 := []Message{
		{Role: "user", Content: "auth auth auth middleware jwt token validation authentication"},
		{Role: "assistant", Content: "completely unrelated content about databases and queries"},
	}

	// Session with less relevant content
	messages2 := []Message{
		{Role: "user", Content: "the auth system needs updating"},
		{Role: "assistant", Content: "weather is nice today"},
	}

	se.IndexSession("high-relevance", messages1)
	se.IndexSession("low-relevance", messages2)

	results := se.Search("auth", SearchOptions{MaxResults: 10})
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// The message with more auth mentions should score higher
	if results[0].Score <= results[1].Score {
		t.Errorf("expected first result to have higher score: %f vs %f", results[0].Score, results[1].Score)
	}
}

func TestSearchRegex(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	messages := []Message{
		{Role: "user", Content: "implement func HandleAuth(w http.ResponseWriter, r *http.Request)"},
		{Role: "assistant", Content: "here is the function implementation"},
		{Role: "user", Content: "add func ValidateToken(token string) bool"},
	}

	se.IndexSession("regex-test", messages)

	results := se.SearchRegex(`func \w+\(`, SearchOptions{MaxResults: 10})
	if len(results) != 2 {
		t.Fatalf("expected 2 regex matches, got %d", len(results))
	}

	// Both function declarations should be found
	for _, r := range results {
		if !strings.Contains(r.Content, "func ") {
			t.Errorf("expected result to contain 'func ', got: %s", r.Content)
		}
	}
}

func TestHighlightPositions(t *testing.T) {
	content := "implement the auth middleware with JWT validation"
	highlights := HighlightMatches(content, "auth")

	if len(highlights) == 0 {
		t.Fatal("expected highlights")
	}

	h := highlights[0]
	if h.Start != 14 {
		t.Errorf("expected highlight start at 14, got %d", h.Start)
	}
	if h.End != 18 {
		t.Errorf("expected highlight end at 18, got %d", h.End)
	}

	// Verify the highlighted text is correct
	highlighted := content[h.Start:h.End]
	if highlighted != "auth" {
		t.Errorf("expected highlighted text 'auth', got '%s'", highlighted)
	}
}

func TestHighlightMultipleOccurrences(t *testing.T) {
	content := "auth middleware with auth validation"
	highlights := HighlightMatches(content, "auth")

	if len(highlights) != 2 {
		t.Fatalf("expected 2 highlights, got %d", len(highlights))
	}

	if highlights[0].Start != 0 || highlights[0].End != 4 {
		t.Errorf("first highlight wrong: %+v", highlights[0])
	}
	if highlights[1].Start != 21 || highlights[1].End != 25 {
		t.Errorf("second highlight wrong: %+v", highlights[1])
	}
}

func TestDateFiltering(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-1 * time.Hour)

	messages := []Message{
		{Role: "user", Content: "old auth discussion"},
		{Role: "user", Content: "recent auth update"},
	}

	se.IndexSession("date-test", messages)

	// Manually set timestamps on indexed messages
	se.mu.Lock()
	se.Index["date-test"].Messages[0].Timestamp = oldTime
	se.Index["date-test"].Messages[1].Timestamp = recentTime
	se.mu.Unlock()

	// Filter to only recent messages (after 24 hours ago)
	results := se.Search("auth", SearchOptions{
		MaxResults: 10,
		DateAfter:  now.Add(-24 * time.Hour),
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result with date filter, got %d", len(results))
	}

	if results[0].MessageIndex != 1 {
		t.Errorf("expected message index 1 (recent), got %d", results[0].MessageIndex)
	}
}

func TestRoleFiltering(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	messages := []Message{
		{Role: "user", Content: "please implement auth middleware"},
		{Role: "assistant", Content: "I'll implement the auth middleware now"},
		{Role: "user", Content: "also add auth to the API routes"},
	}

	se.IndexSession("role-test", messages)

	// Filter to only user messages
	results := se.Search("auth", SearchOptions{
		MaxResults: 10,
		RoleFilter: "user",
	})

	for _, r := range results {
		if r.Role != "user" {
			t.Errorf("expected role 'user', got '%s'", r.Role)
		}
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 user results, got %d", len(results))
	}
}

func TestEmptyQueryReturnsNothing(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	messages := []Message{
		{Role: "user", Content: "some content here"},
	}
	se.IndexSession("empty-test", messages)

	results := se.Search("", SearchOptions{MaxResults: 10})
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}

	results = se.Search("   ", SearchOptions{MaxResults: 10})
	if len(results) != 0 {
		t.Errorf("expected 0 results for whitespace query, got %d", len(results))
	}
}

func TestFormatResults(t *testing.T) {
	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	results := []FTSResult{
		{
			SessionID:    "session-abc123",
			MessageIndex: 0,
			Role:         "assistant",
			Content:      "implement the auth middleware with JWT validation",
			Preview:      "implement the auth middleware with JWT validation",
			Score:        2.5,
			Highlights: []Highlight{
				{Start: 14, End: 18},
			},
			Timestamp: ts,
		},
	}

	output := FormatResults(results, 0)

	if !strings.Contains(output, "session-abc123") {
		t.Error("expected output to contain session ID")
	}
	if !strings.Contains(output, "2024-03-15 10:30") {
		t.Error("expected output to contain timestamp")
	}
	if !strings.Contains(output, "(assistant)") {
		t.Error("expected output to contain role")
	}
	if !strings.Contains(output, "**auth**") {
		t.Error("expected output to contain highlighted term")
	}
}

func TestFormatResultsNoResults(t *testing.T) {
	output := FormatResults(nil, 0)
	if output != "No results found." {
		t.Errorf("expected 'No results found.', got '%s'", output)
	}
}

func TestMultipleSessionsSearched(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	se.IndexSession("session-1", []Message{
		{Role: "user", Content: "implement authentication"},
	})
	se.IndexSession("session-2", []Message{
		{Role: "user", Content: "add authentication to API"},
	})
	se.IndexSession("session-3", []Message{
		{Role: "user", Content: "fix database connection"},
	})

	results := se.Search("authentication", SearchOptions{MaxResults: 10})
	if len(results) != 2 {
		t.Fatalf("expected 2 results from 2 sessions, got %d", len(results))
	}

	// Should find results from both sessions
	sessions := make(map[string]bool)
	for _, r := range results {
		sessions[r.SessionID] = true
	}
	if !sessions["session-1"] || !sessions["session-2"] {
		t.Error("expected results from session-1 and session-2")
	}
}

func TestScoreOrdering(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	// Message with term appearing multiple times should rank higher
	se.IndexSession("multi", []Message{
		{Role: "user", Content: "deploy deploy deploy the application to deploy server"},
	})
	se.IndexSession("single", []Message{
		{Role: "user", Content: "deploy the application"},
	})

	results := se.Search("deploy", SearchOptions{MaxResults: 10})
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Multi-occurrence should score higher
	if results[0].SessionID != "multi" {
		t.Errorf("expected 'multi' session first, got '%s'", results[0].SessionID)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("expected first result to have higher score: %f <= %f", results[0].Score, results[1].Score)
	}
}

func TestCaseInsensitiveSearch(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	messages := []Message{
		{Role: "user", Content: "Implement the Authentication middleware"},
		{Role: "assistant", Content: "AUTHENTICATION is important for security"},
	}

	se.IndexSession("case-test", messages)

	// Lowercase query should find uppercase content
	results := se.Search("authentication", SearchOptions{MaxResults: 10})
	if len(results) != 2 {
		t.Fatalf("expected 2 case-insensitive results, got %d", len(results))
	}

	// Uppercase query should also work
	results = se.Search("AUTHENTICATION", SearchOptions{MaxResults: 10})
	if len(results) != 2 {
		t.Fatalf("expected 2 case-insensitive results for uppercase query, got %d", len(results))
	}
}

func TestSessionFilterSearch(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	se.IndexSession("target", []Message{
		{Role: "user", Content: "implement auth here"},
	})
	se.IndexSession("other", []Message{
		{Role: "user", Content: "implement auth there"},
	})

	results := se.Search("auth", SearchOptions{
		MaxResults:    10,
		SessionFilter: "target",
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result with session filter, got %d", len(results))
	}
	if results[0].SessionID != "target" {
		t.Errorf("expected session 'target', got '%s'", results[0].SessionID)
	}
}

func TestBuildBM25Score(t *testing.T) {
	// Test basic BM25 computation
	queryTerms := []string{"auth"}
	doc := "implement the auth middleware with auth validation"
	docLen := len(doc)
	avgDocLen := float64(50)
	df := map[string]int{"auth": 2}
	totalDocs := 10

	score := BuildBM25Score(queryTerms, doc, docLen, avgDocLen, df, totalDocs)
	if score <= 0 {
		t.Errorf("expected positive BM25 score, got %f", score)
	}

	// Document with more term occurrences should score higher
	docMore := "auth auth auth auth"
	scoreMore := BuildBM25Score(queryTerms, docMore, len(docMore), avgDocLen, df, totalDocs)
	if scoreMore <= score {
		t.Errorf("expected higher score for more occurrences: %f <= %f", scoreMore, score)
	}
}

func TestBuildBM25ScoreZeroCases(t *testing.T) {
	// Zero average doc length
	score := BuildBM25Score([]string{"test"}, "test", 4, 0, map[string]int{}, 10)
	if score != 0 {
		t.Errorf("expected 0 score with zero avgDocLen, got %f", score)
	}

	// Zero total docs
	score = BuildBM25Score([]string{"test"}, "test", 4, 50, map[string]int{}, 0)
	if score != 0 {
		t.Errorf("expected 0 score with zero totalDocs, got %f", score)
	}

	// Term not in document
	score = BuildBM25Score([]string{"missing"}, "test content", 12, 50, map[string]int{"missing": 1}, 10)
	if score != 0 {
		t.Errorf("expected 0 score for missing term, got %f", score)
	}
}

func TestSearchRegexInvalidPattern(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	se.IndexSession("regex-invalid", []Message{
		{Role: "user", Content: "some content"},
	})

	// Invalid regex should return nil
	results := se.SearchRegex("[invalid", SearchOptions{MaxResults: 10})
	if results != nil {
		t.Errorf("expected nil for invalid regex, got %v", results)
	}
}

func TestRebuildIndex(t *testing.T) {
	// Create a temp directory with session files
	dir := t.TempDir()

	// Write a JSONL session file
	sessionFile := filepath.Join(dir, "rebuild-test.jsonl")
	meta := `{"type":"session_meta","id":"rebuild-test","model":"claude-3","provider":"anthropic","created_at":"2024-03-15T10:00:00Z"}` + "\n"
	msg1 := `{"role":"user","content":"search for this content"}` + "\n"
	msg2 := `{"role":"assistant","content":"found the search results"}` + "\n"
	if err := os.WriteFile(sessionFile, []byte(meta+msg1+msg2), 0o644); err != nil {
		t.Fatal(err)
	}

	se := NewSearchEngine(dir)
	if err := se.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex failed: %v", err)
	}

	// Verify the session was indexed
	se.mu.RLock()
	idx, ok := se.Index["rebuild-test"]
	se.mu.RUnlock()

	if !ok {
		t.Fatal("expected session 'rebuild-test' in index")
	}
	if idx.Model != "claude-3" {
		t.Errorf("expected model 'claude-3', got '%s'", idx.Model)
	}
	if idx.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", idx.Provider)
	}
	if len(idx.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(idx.Messages))
	}

	// Search should find content
	results := se.Search("search", SearchOptions{MaxResults: 10})
	if len(results) == 0 {
		t.Error("expected search results after rebuild")
	}
}

func TestRebuildIndexNonexistentDir(t *testing.T) {
	se := NewSearchEngine("/nonexistent/path/that/does/not/exist")
	err := se.RebuildIndex()
	if err != nil {
		t.Errorf("expected nil error for nonexistent dir, got: %v", err)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"func_name(arg)", []string{"func", "name", "arg"}},
		{"camelCase", []string{"camelcase"}},
		{"HTTP/2.0", []string{"http", "2", "0"}},
		{"", nil},
		{"   ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("tokenize(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSearchWithMaxResults(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	// Create many matching messages
	var messages []Message
	for i := 0; i < 50; i++ {
		messages = append(messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("deploy application number %d to production", i),
		})
	}
	se.IndexSession("many-results", messages)

	results := se.Search("deploy", SearchOptions{MaxResults: 5})
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestSearchRegexRoleFilter(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	messages := []Message{
		{Role: "user", Content: "func main() {}"},
		{Role: "assistant", Content: "func helper() {}"},
	}
	se.IndexSession("regex-role", messages)

	results := se.SearchRegex(`func \w+`, SearchOptions{
		MaxResults: 10,
		RoleFilter: "assistant",
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result with role filter, got %d", len(results))
	}
	if results[0].Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", results[0].Role)
	}
}

func TestHighlightMatchesCaseInsensitive(t *testing.T) {
	content := "Implement AUTH middleware"
	highlights := HighlightMatches(content, "auth")

	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}

	// Should find "AUTH" at position 10-14
	if highlights[0].Start != 10 || highlights[0].End != 14 {
		t.Errorf("highlight at wrong position: %+v", highlights[0])
	}
}

func TestHighlightMatchesMultipleTerms(t *testing.T) {
	content := "implement auth middleware for jwt"
	highlights := HighlightMatches(content, "auth jwt")

	if len(highlights) != 2 {
		t.Fatalf("expected 2 highlights, got %d", len(highlights))
	}

	// auth at 10-14, jwt at 30-33
	if highlights[0].Start != 10 || highlights[0].End != 14 {
		t.Errorf("first highlight wrong: %+v", highlights[0])
	}
	if highlights[1].Start != 30 || highlights[1].End != 33 {
		t.Errorf("second highlight wrong: %+v", highlights[1])
	}
}

func TestFormatResultsWithContext(t *testing.T) {
	results := []FTSResult{
		{
			SessionID:    "ctx-test",
			MessageIndex: 0,
			Role:         "user",
			Content:      "a very long content string that should be truncated when context is limited",
			Preview:      "a very long content string that should be truncated when context is limited",
			Score:        1.0,
			Highlights:   nil,
			Timestamp:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	output := FormatResults(results, 20)
	// Should contain truncated content
	if !strings.Contains(output, "a very long content ") {
		t.Error("expected truncated content in output")
	}
}

func TestIndexSessionPreviewTruncation(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	longContent := strings.Repeat("a", 200)
	messages := []Message{
		{Role: "user", Content: longContent},
	}

	se.IndexSession("preview-test", messages)

	se.mu.RLock()
	idx := se.Index["preview-test"]
	se.mu.RUnlock()

	if len(idx.Messages[0].Preview) != 100 {
		t.Errorf("expected preview length 100, got %d", len(idx.Messages[0].Preview))
	}
}

func TestConcurrentAccess(t *testing.T) {
	se := NewSearchEngine("/tmp/sessions")

	// Index some data first
	se.IndexSession("concurrent", []Message{
		{Role: "user", Content: "concurrent access test content"},
	})

	// Concurrent reads should not panic
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = se.Search("test", SearchOptions{MaxResults: 5})
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
