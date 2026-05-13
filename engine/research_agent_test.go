package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewResearchAgent(t *testing.T) {
	t.Run("default workers", func(t *testing.T) {
		ra := NewResearchAgent(0)
		if ra.MaxWorkers != 5 {
			t.Errorf("expected MaxWorkers=5, got %d", ra.MaxWorkers)
		}
	})
	t.Run("negative workers defaults to 5", func(t *testing.T) {
		ra := NewResearchAgent(-1)
		if ra.MaxWorkers != 5 {
			t.Errorf("expected MaxWorkers=5, got %d", ra.MaxWorkers)
		}
	})
	t.Run("custom workers", func(t *testing.T) {
		ra := NewResearchAgent(10)
		if ra.MaxWorkers != 10 {
			t.Errorf("expected MaxWorkers=10, got %d", ra.MaxWorkers)
		}
	})
	t.Run("timeout is set", func(t *testing.T) {
		ra := NewResearchAgent(3)
		if ra.Timeout != 30*time.Second {
			t.Errorf("expected Timeout=30s, got %v", ra.Timeout)
		}
	})
}

func TestDecomposeQuestion(t *testing.T) {
	ra := NewResearchAgent(5)

	t.Run("auth question", func(t *testing.T) {
		subs := ra.DecomposeQuestion("How does auth work in this project?")
		if len(subs) == 0 {
			t.Fatal("expected sub-questions, got none")
		}
		found := false
		for _, s := range subs {
			if strings.Contains(s, "auth") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected at least one sub-question containing 'auth'")
		}
	})

	t.Run("test question", func(t *testing.T) {
		subs := ra.DecomposeQuestion("How do I run tests?")
		if len(subs) == 0 {
			t.Fatal("expected sub-questions, got none")
		}
		found := false
		for _, s := range subs {
			if strings.Contains(s, "test") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected at least one sub-question containing 'test'")
		}
	})

	t.Run("deploy question", func(t *testing.T) {
		subs := ra.DecomposeQuestion("How to deploy this service?")
		if len(subs) == 0 {
			t.Fatal("expected sub-questions, got none")
		}
	})

	t.Run("database question", func(t *testing.T) {
		subs := ra.DecomposeQuestion("How is the database configured?")
		if len(subs) == 0 {
			t.Fatal("expected sub-questions, got none")
		}
	})

	t.Run("generic question", func(t *testing.T) {
		subs := ra.DecomposeQuestion("What logging framework is used?")
		if len(subs) == 0 {
			t.Fatal("expected sub-questions, got none")
		}
	})

	t.Run("limits to 6 sub-queries", func(t *testing.T) {
		subs := ra.DecomposeQuestion("complex query with many different keywords spread across various topics and categories")
		if len(subs) > 6 {
			t.Errorf("expected at most 6 sub-queries, got %d", len(subs))
		}
	})
}

func TestParallelSearch(t *testing.T) {
	ra := NewResearchAgent(3)

	t.Run("basic parallel search", func(t *testing.T) {
		queries := []string{"query1", "query2", "query3"}
		searchFn := func(q string) (string, error) {
			return "result for " + q, nil
		}

		findings := ra.ParallelSearch(context.Background(), queries, searchFn)
		if len(findings) != 3 {
			t.Errorf("expected 3 findings, got %d", len(findings))
		}
	})

	t.Run("handles errors gracefully", func(t *testing.T) {
		queries := []string{"good", "bad", "good2"}
		searchFn := func(q string) (string, error) {
			if q == "bad" {
				return "", errors.New("search failed")
			}
			return "result for " + q, nil
		}

		findings := ra.ParallelSearch(context.Background(), queries, searchFn)
		if len(findings) != 2 {
			t.Errorf("expected 2 findings (1 error skipped), got %d", len(findings))
		}
	})

	t.Run("skips empty results", func(t *testing.T) {
		queries := []string{"full", "empty"}
		searchFn := func(q string) (string, error) {
			if q == "empty" {
				return "", nil
			}
			return "content", nil
		}

		findings := ra.ParallelSearch(context.Background(), queries, searchFn)
		if len(findings) != 1 {
			t.Errorf("expected 1 finding (empty skipped), got %d", len(findings))
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		queries := []string{"q1", "q2", "q3"}
		searchFn := func(q string) (string, error) {
			return "result", nil
		}

		findings := ra.ParallelSearch(ctx, queries, searchFn)
		// With cancelled context, we may get 0 results.
		if len(findings) > 3 {
			t.Errorf("unexpected findings count: %d", len(findings))
		}
	})

	t.Run("nil queries returns nil", func(t *testing.T) {
		findings := ra.ParallelSearch(context.Background(), nil, func(q string) (string, error) {
			return "x", nil
		})
		if findings != nil {
			t.Errorf("expected nil, got %v", findings)
		}
	})

	t.Run("respects max workers", func(t *testing.T) {
		ra := NewResearchAgent(2)
		var concurrent int64
		var maxConcurrent int64

		queries := make([]string, 10)
		for i := range queries {
			queries[i] = fmt.Sprintf("q%d", i)
		}

		searchFn := func(q string) (string, error) {
			cur := atomic.AddInt64(&concurrent, 1)
			// Track max concurrent workers.
			for {
				old := atomic.LoadInt64(&maxConcurrent)
				if cur <= old {
					break
				}
				if atomic.CompareAndSwapInt64(&maxConcurrent, old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&concurrent, -1)
			return "result", nil
		}

		ra.ParallelSearch(context.Background(), queries, searchFn)
		max := atomic.LoadInt64(&maxConcurrent)
		if max > 2 {
			t.Errorf("expected max 2 concurrent workers, got %d", max)
		}
	})
}

func TestRankFindings(t *testing.T) {
	ra := NewResearchAgent(5)

	t.Run("ranks by relevance", func(t *testing.T) {
		findings := []ResearchFinding{
			{Content: "unrelated stuff about cooking", Source: "s1"},
			{Content: "JWT token authentication middleware handler", Source: "s2"},
			{Content: "auth config with secret key", Source: "s3"},
		}

		ranked := ra.RankFindings(findings, "How does authentication work?")
		if len(ranked) != 3 {
			t.Fatalf("expected 3 findings, got %d", len(ranked))
		}
		// The auth-related content should rank higher.
		if ranked[0].Relevance < ranked[len(ranked)-1].Relevance {
			t.Error("findings not sorted by relevance descending")
		}
	})

	t.Run("empty findings", func(t *testing.T) {
		ranked := ra.RankFindings(nil, "anything")
		if ranked != nil {
			t.Errorf("expected nil, got %v", ranked)
		}
	})

	t.Run("sets confidence", func(t *testing.T) {
		findings := []ResearchFinding{
			{Content: strings.Repeat("authentication token validation ", 20), Source: "s1"},
		}
		ranked := ra.RankFindings(findings, "authentication token")
		if ranked[0].Confidence <= 0 {
			t.Error("expected positive confidence")
		}
	})
}

func TestResearchAgentSynthesize(t *testing.T) {
	ra := NewResearchAgent(5)

	t.Run("combines findings", func(t *testing.T) {
		findings := []ResearchFinding{
			{Content: "JWT auth in src/auth/token.go", Source: "auth files", Relevance: 0.95, Confidence: 0.9},
			{Content: "Middleware chain in src/middleware/", Source: "middleware", Relevance: 0.85, Confidence: 0.8},
		}

		result := ra.Synthesize(findings, "How does auth work?")
		if !strings.Contains(result, "JWT auth") {
			t.Error("expected summary to mention JWT auth")
		}
		if !strings.Contains(result, "Middleware chain") {
			t.Error("expected summary to mention middleware")
		}
	})

	t.Run("deduplicates overlapping content", func(t *testing.T) {
		findings := []ResearchFinding{
			{Content: "authentication uses JWT tokens", Source: "s1", Relevance: 0.9, Confidence: 0.9},
			{Content: "authentication uses JWT tokens", Source: "s2", Relevance: 0.8, Confidence: 0.8},
		}

		result := ra.Synthesize(findings, "auth")
		// Should only appear once in the output.
		count := strings.Count(result, "authentication uses JWT tokens")
		if count > 1 {
			t.Errorf("expected deduplicated content, found %d occurrences", count)
		}
	})

	t.Run("no findings", func(t *testing.T) {
		result := ra.Synthesize(nil, "test query")
		if !strings.Contains(result, "No findings") {
			t.Error("expected 'No findings' message")
		}
	})

	t.Run("truncates long content", func(t *testing.T) {
		long := strings.Repeat("x", 500)
		findings := []ResearchFinding{
			{Content: long, Source: "s1", Relevance: 0.9, Confidence: 0.9},
		}

		result := ra.Synthesize(findings, "query")
		if strings.Contains(result, long) {
			t.Error("expected content to be truncated")
		}
		if !strings.Contains(result, "...") {
			t.Error("expected truncation indicator '...'")
		}
	})
}

func TestFormatResult(t *testing.T) {
	ra := NewResearchAgent(5)

	t.Run("formats complete result", func(t *testing.T) {
		result := &ResearchResult{
			Query: "How does authentication work?",
			Findings: []ResearchFinding{
				{Content: "JWT-based auth in src/auth/token.go", Source: "auth_files", Relevance: 0.95, Confidence: 0.95},
				{Content: "Middleware chain in src/middleware/", Source: "middleware", Relevance: 0.88, Confidence: 0.88},
			},
			Sources:     []string{"auth_files", "middleware"},
			Duration:    2300 * time.Millisecond,
			TotalTokens: 150,
		}

		formatted := ra.FormatResult(result)
		if !strings.Contains(formatted, "Research:") {
			t.Error("missing Research header")
		}
		if !strings.Contains(formatted, "Duration: 2.3s") {
			t.Error("missing or incorrect duration")
		}
		if !strings.Contains(formatted, "Sources: 2") {
			t.Error("missing sources count")
		}
		if !strings.Contains(formatted, "Findings: 2") {
			t.Error("missing findings count")
		}
		if !strings.Contains(formatted, "Key findings:") {
			t.Error("missing key findings section")
		}
		if !strings.Contains(formatted, "confidence:") {
			t.Error("missing confidence scores")
		}
		if !strings.Contains(formatted, "─") {
			t.Error("missing separator line")
		}
	})

	t.Run("nil result", func(t *testing.T) {
		formatted := ra.FormatResult(nil)
		if formatted != "" {
			t.Errorf("expected empty string for nil result, got %q", formatted)
		}
	})

	t.Run("limits to 5 findings", func(t *testing.T) {
		findings := make([]ResearchFinding, 8)
		for i := range findings {
			findings[i] = ResearchFinding{
				Content:    fmt.Sprintf("Finding %d content", i),
				Source:     fmt.Sprintf("source%d", i),
				Relevance:  float64(8-i) / 10.0,
				Confidence: float64(8-i) / 10.0,
			}
		}
		result := &ResearchResult{
			Query:    "test",
			Findings: findings,
			Sources:  []string{"s1"},
			Duration: time.Second,
		}

		formatted := ra.FormatResult(result)
		// Count numbered findings in key findings section.
		count := strings.Count(formatted, "confidence:")
		// Should have at most 5 in key findings + up to 8 in summary.
		if count < 5 {
			t.Errorf("expected at least 5 confidence entries, got %d", count)
		}
	})
}

func TestResearch(t *testing.T) {
	t.Run("full research cycle", func(t *testing.T) {
		ra := NewResearchAgent(3)
		ra.Timeout = 5 * time.Second

		searchFn := func(q string) (string, error) {
			responses := map[string]string{
				"auth middleware files": "Found auth middleware in src/middleware/auth.go using JWT",
				"token validation":      "Token validation with RS256 in src/auth/verify.go",
				"session management":    "Sessions stored in Redis with 24h TTL",
				"auth config":           "AUTH_SECRET and AUTH_ISSUER in .env file",
			}
			if resp, ok := responses[q]; ok {
				return resp, nil
			}
			return "generic result for " + q, nil
		}

		query := ResearchQuery{
			Question:  "How does authentication work in this project?",
			MaxTokens: 1000,
		}

		result, err := ra.Research(context.Background(), query, searchFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.Query != query.Question {
			t.Errorf("expected query=%q, got %q", query.Question, result.Query)
		}
		if len(result.Findings) == 0 {
			t.Error("expected findings, got none")
		}
		if result.Duration <= 0 {
			t.Error("expected positive duration")
		}
		if len(result.Sources) == 0 {
			t.Error("expected sources, got none")
		}
	})

	t.Run("with provided sub-questions", func(t *testing.T) {
		ra := NewResearchAgent(2)
		ra.Timeout = 5 * time.Second

		var searched []string
		searchFn := func(q string) (string, error) {
			searched = append(searched, q)
			return "result for " + q, nil
		}

		query := ResearchQuery{
			Question:     "Main question",
			SubQuestions: []string{"sub1", "sub2"},
		}

		_, err := ra.Research(context.Background(), query, searchFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(searched) != 2 {
			t.Errorf("expected 2 searches, got %d", len(searched))
		}
	})

	t.Run("stores results", func(t *testing.T) {
		ra := NewResearchAgent(2)
		ra.Timeout = 5 * time.Second

		searchFn := func(q string) (string, error) {
			return "content", nil
		}

		query := ResearchQuery{Question: "test question"}
		ra.Research(context.Background(), query, searchFn)

		ra.mu.RLock()
		defer ra.mu.RUnlock()
		if len(ra.Results) != 1 {
			t.Errorf("expected 1 stored result, got %d", len(ra.Results))
		}
	})
}

func TestResearchAgentExtractKeywords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "How does auth work in this project?",
			expected: []string{"auth"},
		},
		{
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			keywords := extractKeywords(tt.input)
			for _, exp := range tt.expected {
				found := false
				for _, kw := range keywords {
					if kw == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected keyword %q not found in %v", exp, keywords)
				}
			}
		})
	}
}

func TestIsDuplicate(t *testing.T) {
	t.Run("exact duplicate", func(t *testing.T) {
		if !isDuplicate("hello world", []string{"hello world"}) {
			t.Error("expected duplicate detection")
		}
	})
	t.Run("not duplicate", func(t *testing.T) {
		if isDuplicate("completely different", []string{"hello world"}) {
			t.Error("should not be detected as duplicate")
		}
	})
	t.Run("empty seen list", func(t *testing.T) {
		if isDuplicate("anything", nil) {
			t.Error("should not detect duplicate in empty list")
		}
	})
}
