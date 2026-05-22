package search

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIssueTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple words",
			input:    "token expiry causes 500 error",
			expected: []string{"token", "expiry", "causes", "500", "error"},
		},
		{
			name:     "filters stop words",
			input:    "the server is not responding to the request",
			expected: []string{"server", "responding", "request"},
		},
		{
			name:     "handles special characters",
			input:    "nil-pointer in handler.go (line 42)",
			expected: []string{"nil", "pointer", "handler", "go", "line", "42"},
		},
		{
			name:     "deduplicates tokens",
			input:    "error error error different",
			expected: []string{"error", "different"},
		},
		{
			name:     "filters short tokens",
			input:    "a b c de fg hi",
			expected: []string{"de", "fg", "hi"},
		},
		{
			name:     "case insensitive",
			input:    "Token EXPIRY Causes ERROR",
			expected: []string{"token", "expiry", "causes", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := issueTokenize(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("issueTokenize(%q) = %v (len %d), want %v (len %d)",
					tt.input, result, len(result), tt.expected, len(tt.expected))
				return
			}
			for i, tok := range result {
				if tok != tt.expected[i] {
					t.Errorf("issueTokenize(%q)[%d] = %q, want %q",
						tt.input, i, tok, tt.expected[i])
				}
			}
		})
	}
}

func TestNewIssueIndex(t *testing.T) {
	idx := NewIssueIndex()
	if idx == nil {
		t.Fatal("NewIssueIndex returned nil")
	}
	if len(idx.Issues) != 0 {
		t.Errorf("expected empty issues, got %d", len(idx.Issues))
	}
	if idx.InvertedIndex == nil {
		t.Error("InvertedIndex should not be nil")
	}
}

func TestAddIssue(t *testing.T) {
	idx := NewIssueIndex()

	issue := &Issue{
		ID:    "42",
		Title: "Token expiry causes 500 error",
		Body:  "When the JWT token expires, the auth middleware returns a 500 instead of 401",
		State: "closed",
	}

	idx.AddIssue(issue)

	if len(idx.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(idx.Issues))
	}

	if len(issue.Tokens) == 0 {
		t.Error("issue tokens should be populated after AddIssue")
	}

	// Check inverted index has entries
	if len(idx.InvertedIndex) == 0 {
		t.Error("inverted index should have entries")
	}

	// Token "token" should be indexed
	if docs, ok := idx.InvertedIndex["token"]; !ok || len(docs) == 0 {
		t.Error("expected 'token' to be in inverted index")
	}

	// Token "500" should be indexed
	if docs, ok := idx.InvertedIndex["500"]; !ok || len(docs) == 0 {
		t.Error("expected '500' to be in inverted index")
	}
}

func TestFindSimilar(t *testing.T) {
	idx := NewIssueIndex()

	// Add several issues
	idx.AddIssue(&Issue{
		ID:         "42",
		Title:      "Token expiry causes 500 error",
		Body:       "When the JWT token expires, the auth middleware returns a 500 instead of 401",
		State:      "closed",
		Resolution: "Added token refresh before expiry check",
	})

	idx.AddIssue(&Issue{
		ID:         "38",
		Title:      "Auth middleware race condition",
		Body:       "Concurrent requests cause race condition in shared auth state",
		State:      "closed",
		Resolution: "Added mutex to shared state",
	})

	idx.AddIssue(&Issue{
		ID:    "55",
		Title: "JWT validation fails on RS256",
		Body:  "JWT tokens signed with RS256 algorithm fail validation",
		State: "open",
	})

	idx.AddIssue(&Issue{
		ID:    "60",
		Title: "Database connection pool exhaustion",
		Body:  "Under high load, the database connection pool is exhausted causing timeouts",
		State: "open",
	})

	// Search for token-related issues
	results := idx.FindSimilar("JWT token expired returns wrong status code", 3)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// Issue #42 should be the most similar (token, expired, 500)
	if results[0].Issue.ID != "42" {
		t.Errorf("expected issue #42 as top result, got #%s", results[0].Issue.ID)
	}

	// Should have matching terms
	if len(results[0].MatchingTerms) == 0 {
		t.Error("expected matching terms in result")
	}

	// Score should be positive
	if results[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", results[0].Score)
	}

	// Database issue should NOT be in top results for token query
	for _, r := range results {
		if r.Issue.ID == "60" {
			t.Error("database issue should not match token query")
		}
	}
}

func TestFindSimilarEmptyIndex(t *testing.T) {
	idx := NewIssueIndex()
	results := idx.FindSimilar("some query", 5)
	if results != nil {
		t.Errorf("expected nil results for empty index, got %v", results)
	}
}

func TestFindSimilarEmptyQuery(t *testing.T) {
	idx := NewIssueIndex()
	idx.AddIssue(&Issue{
		ID:    "1",
		Title: "Some issue",
		Body:  "Some body text",
		State: "open",
	})

	results := idx.FindSimilar("", 5)
	if results != nil {
		t.Errorf("expected nil results for empty query, got %v", results)
	}
}

func TestFindSimilarLimit(t *testing.T) {
	idx := NewIssueIndex()

	// Add 10 issues all containing "error"
	for i := 0; i < 10; i++ {
		idx.AddIssue(&Issue{
			ID:    fmt.Sprintf("%d", i),
			Title: fmt.Sprintf("Error scenario %d", i),
			Body:  "This is an error that needs fixing",
			State: "open",
		})
	}

	results := idx.FindSimilar("error fixing", 3)
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

func TestSuggestResolution(t *testing.T) {
	similar := []*SimilarIssue{
		{
			Issue: &Issue{
				ID:         "42",
				Title:      "Token expiry error",
				State:      "closed",
				Resolution: "Added token refresh before expiry check",
			},
			Score: 0.9,
		},
		{
			Issue: &Issue{
				ID:    "55",
				Title: "JWT validation fails",
				State: "open",
			},
			Score: 0.6,
		},
	}

	result := SuggestResolution(similar)

	if !strings.Contains(result, "#42") {
		t.Error("expected suggestion to reference issue #42")
	}
	if !strings.Contains(result, "token refresh") {
		t.Error("expected suggestion to mention the resolution")
	}
}

func TestSuggestResolutionNoResolved(t *testing.T) {
	similar := []*SimilarIssue{
		{
			Issue: &Issue{
				ID:    "55",
				Title: "Open issue",
				State: "open",
			},
			Score: 0.6,
		},
	}

	result := SuggestResolution(similar)
	if !strings.Contains(result, "No resolved similar issues") {
		t.Errorf("expected no resolution message, got: %s", result)
	}
}

func TestSuggestResolutionEmpty(t *testing.T) {
	result := SuggestResolution(nil)
	if !strings.Contains(result, "No similar issues found") {
		t.Errorf("expected no similar issues message, got: %s", result)
	}
}

func TestFormatIssueResults(t *testing.T) {
	similar := []*SimilarIssue{
		{
			Issue: &Issue{
				ID:         "42",
				Title:      "Token expiry causes 500 error",
				State:      "closed",
				Resolution: "Added token refresh before expiry check",
			},
			Score:         2.5,
			MatchingTerms: []string{"token", "expiry", "500", "error"},
		},
		{
			Issue: &Issue{
				ID:         "38",
				Title:      "Auth middleware race condition",
				State:      "closed",
				Resolution: "Added mutex to shared state",
			},
			Score:         1.8,
			MatchingTerms: []string{"auth", "middleware"},
		},
		{
			Issue: &Issue{
				ID:    "55",
				Title: "JWT validation fails on RS256",
				State: "open",
			},
			Score:         1.2,
			MatchingTerms: []string{"jwt", "validation"},
		},
	}

	result := FormatIssueResults(similar)

	// Check header
	if !strings.Contains(result, "Similar Issues Found:") {
		t.Error("expected header in output")
	}

	// Check issue entries
	if !strings.Contains(result, "#42") {
		t.Error("expected issue #42 in output")
	}
	if !strings.Contains(result, "#38") {
		t.Error("expected issue #38 in output")
	}
	if !strings.Contains(result, "#55") {
		t.Error("expected issue #55 in output")
	}

	// Check state labels
	if !strings.Contains(result, "CLOSED") {
		t.Error("expected CLOSED label in output")
	}
	if !strings.Contains(result, "OPEN") {
		t.Error("expected OPEN label in output")
	}

	// Check resolution
	if !strings.Contains(result, "Resolution: Added token refresh") {
		t.Error("expected resolution text in output")
	}

	// Check "No resolution yet" for open issues
	if !strings.Contains(result, "No resolution yet") {
		t.Error("expected 'No resolution yet' for open issue")
	}

	// Check matching terms
	if !strings.Contains(result, "\"token\"") {
		t.Error("expected quoted matching term in output")
	}
}

func TestFormatIssueResultsEmpty(t *testing.T) {
	result := FormatIssueResults(nil)
	if result != "No similar issues found." {
		t.Errorf("expected empty message, got: %s", result)
	}
}

func TestBuildSearchContext(t *testing.T) {
	similar := []*SimilarIssue{
		{
			Issue: &Issue{
				ID:         "42",
				Title:      "Token expiry causes 500 error",
				Body:       "When the JWT token expires, the auth middleware returns a 500",
				Labels:     []string{"bug", "auth"},
				State:      "closed",
				Resolution: "Added token refresh before expiry check",
			},
			Score:         2.5,
			MatchingTerms: []string{"token", "expiry"},
		},
	}

	result := BuildSearchContext(similar)

	// Check structure
	if !strings.Contains(result, "## Related Issues Context") {
		t.Error("expected markdown header")
	}
	if !strings.Contains(result, "### Issue #42") {
		t.Error("expected issue header")
	}
	if !strings.Contains(result, "[CLOSED]") {
		t.Error("expected state in header")
	}
	if !strings.Contains(result, "Labels: bug, auth") {
		t.Error("expected labels")
	}
	if !strings.Contains(result, "Resolution: Added token refresh") {
		t.Error("expected resolution")
	}
	if !strings.Contains(result, "consider applying the same fix pattern") {
		t.Error("expected guidance text")
	}
}

func TestBuildSearchContextEmpty(t *testing.T) {
	result := BuildSearchContext(nil)
	if result != "" {
		t.Errorf("expected empty string for nil input, got: %s", result)
	}
}

func TestBuildSearchContextTruncatesLongBody(t *testing.T) {
	longBody := strings.Repeat("word ", 100) // 500 chars
	similar := []*SimilarIssue{
		{
			Issue: &Issue{
				ID:    "1",
				Title: "Long issue",
				Body:  longBody,
				State: "open",
			},
			Score:         1.0,
			MatchingTerms: []string{"word"},
		},
	}

	result := BuildSearchContext(similar)
	if !strings.Contains(result, "...") {
		t.Error("expected truncation indicator for long body")
	}
}

func TestIssueConcurrentAccess(t *testing.T) {
	idx := NewIssueIndex()

	// Add issues concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			idx.AddIssue(&Issue{
				ID:    fmt.Sprintf("%d", n),
				Title: fmt.Sprintf("Issue %d with error handling", n),
				Body:  "Some description about error handling in the system",
				State: "open",
			})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Search concurrently
	for i := 0; i < 10; i++ {
		go func() {
			results := idx.FindSimilar("error handling", 5)
			if len(results) == 0 {
				t.Error("expected results from concurrent search")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(idx.Issues) != 10 {
		t.Errorf("expected 10 issues after concurrent adds, got %d", len(idx.Issues))
	}
}

func TestIssueFields(t *testing.T) {
	now := time.Now()
	closedAt := now.Add(24 * time.Hour)

	issue := &Issue{
		ID:         "99",
		Title:      "Test issue",
		Body:       "Test body",
		Labels:     []string{"bug", "priority"},
		State:      "closed",
		Resolution: "Fixed the thing",
		CreatedAt:  now,
		ClosedAt:   &closedAt,
	}

	if issue.ID != "99" {
		t.Error("ID mismatch")
	}
	if issue.State != "closed" {
		t.Error("State mismatch")
	}
	if issue.ClosedAt == nil || !issue.ClosedAt.Equal(closedAt) {
		t.Error("ClosedAt mismatch")
	}
	if len(issue.Labels) != 2 {
		t.Error("Labels mismatch")
	}
}

func TestBM25Scoring(t *testing.T) {
	idx := NewIssueIndex()

	// Issue with high relevance - many matching terms
	idx.AddIssue(&Issue{
		ID:    "1",
		Title: "Authentication token refresh failure",
		Body:  "The authentication token refresh mechanism fails when the token has expired",
		State: "closed",
	})

	// Issue with medium relevance - some matching terms
	idx.AddIssue(&Issue{
		ID:    "2",
		Title: "Token parsing issue",
		Body:  "Cannot parse malformed tokens",
		State: "open",
	})

	// Issue with low relevance - few matching terms
	idx.AddIssue(&Issue{
		ID:    "3",
		Title: "Database migration failure",
		Body:  "The database migration script fails on large datasets",
		State: "closed",
	})

	results := idx.FindSimilar("authentication token refresh expired", 3)

	if len(results) < 2 {
		t.Fatal("expected at least 2 results")
	}

	// First result should be #1 (most relevant)
	if results[0].Issue.ID != "1" {
		t.Errorf("expected issue #1 as top result, got #%s", results[0].Issue.ID)
	}

	// Scores should be in descending order
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not in descending score order: %f > %f",
				results[i].Score, results[i-1].Score)
		}
	}
}

func TestDedupStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b", "d"}
	result := dedupStrings(input)
	expected := []string{"a", "b", "c", "d"}

	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("dedupStrings[%d] = %q, want %q", i, v, expected[i])
		}
	}
}
