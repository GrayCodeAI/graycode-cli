package search

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ResearchAgent gathers information from multiple sources in parallel,
// inspired by gpt-researcher's parallel crawler pattern.
type ResearchAgent struct {
	MaxWorkers int
	Timeout    time.Duration
	Results    []ResearchResult
	mu         sync.RWMutex
}

// ResearchQuery defines a research task with a main question and optional sub-questions.
type ResearchQuery struct {
	Question     string
	SubQuestions []string
	Sources      []string
	MaxTokens    int
}

// ResearchResult holds the aggregated output of a research operation.
type ResearchResult struct {
	Query       string
	Findings    []ResearchFinding
	Sources     []string
	Duration    time.Duration
	TotalTokens int
}

// ResearchFinding represents a single piece of discovered information.
type ResearchFinding struct {
	Content    string
	Source     string
	Relevance  float64
	Confidence float64
}

// NewResearchAgent creates a ResearchAgent with the given worker pool size.
// If maxWorkers <= 0, it defaults to 5.
func NewResearchAgent(maxWorkers int) *ResearchAgent {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}
	return &ResearchAgent{
		MaxWorkers: maxWorkers,
		Timeout:    30 * time.Second,
	}
}

// Research executes a full research cycle: decompose, search in parallel, rank, and synthesize.
func (ra *ResearchAgent) Research(ctx context.Context, query ResearchQuery, searchFn func(string) (string, error)) (*ResearchResult, error) {
	start := time.Now()

	// Apply timeout if set.
	if ra.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ra.Timeout)
		defer cancel()
	}

	// Decompose the question into sub-queries.
	subQueries := query.SubQuestions
	if len(subQueries) == 0 {
		subQueries = ra.DecomposeQuestion(query.Question)
	}

	// Search all sub-queries in parallel.
	findings := ra.ParallelSearch(ctx, subQueries, searchFn)

	// Rank findings by relevance to the original question.
	ranked := ra.RankFindings(findings, query.Question)

	// Collect unique sources.
	sourceSet := make(map[string]struct{})
	for _, f := range ranked {
		if f.Source != "" {
			sourceSet[f.Source] = struct{}{}
		}
	}
	sources := make([]string, 0, len(sourceSet))
	for s := range sourceSet {
		sources = append(sources, s)
	}
	sort.Strings(sources)

	// Calculate total tokens (approximate by content length / 4).
	totalTokens := 0
	for _, f := range ranked {
		totalTokens += len(f.Content) / 4
	}
	if query.MaxTokens > 0 && totalTokens > query.MaxTokens {
		totalTokens = query.MaxTokens
	}

	result := &ResearchResult{
		Query:       query.Question,
		Findings:    ranked,
		Sources:     sources,
		Duration:    time.Since(start),
		TotalTokens: totalTokens,
	}

	// Store result.
	ra.mu.Lock()
	ra.Results = append(ra.Results, *result)
	ra.mu.Unlock()

	return result, nil
}

// DecomposeQuestion breaks a complex question into searchable sub-queries.
func (ra *ResearchAgent) DecomposeQuestion(question string) []string {
	q := strings.ToLower(question)

	// Pattern-based decomposition for common question types.
	if strings.Contains(q, "auth") {
		return []string{
			"auth middleware files",
			"token validation",
			"session management",
			"auth config",
		}
	}
	if strings.Contains(q, "test") && strings.Contains(q, "how") {
		return []string{
			"test framework setup",
			"test helpers and utilities",
			"test configuration",
			"integration tests",
		}
	}
	if strings.Contains(q, "deploy") {
		return []string{
			"deployment configuration",
			"CI/CD pipeline",
			"environment variables",
			"infrastructure files",
		}
	}
	if strings.Contains(q, "database") || strings.Contains(q, "db") {
		return []string{
			"database connection setup",
			"migration files",
			"database models",
			"query patterns",
		}
	}
	if strings.Contains(q, "api") && (strings.Contains(q, "endpoint") || strings.Contains(q, "route")) {
		return []string{
			"API route definitions",
			"request handlers",
			"middleware chain",
			"API documentation",
		}
	}

	// Generic decomposition: extract key terms and build sub-queries.
	words := researchExtractKeywords(question)
	if len(words) == 0 {
		return []string{question}
	}

	subQueries := make([]string, 0, len(words)+1)
	subQueries = append(subQueries, question)
	for _, w := range words {
		if len(w) > 2 {
			subQueries = append(subQueries, w+" files")
		}
	}
	if len(subQueries) > 6 {
		subQueries = subQueries[:6]
	}
	return subQueries
}

// ParallelSearch runs searches concurrently using a worker pool and collects results.
func (ra *ResearchAgent) ParallelSearch(ctx context.Context, queries []string, searchFn func(string) (string, error)) []ResearchFinding {
	if len(queries) == 0 {
		return nil
	}

	type result struct {
		query   string
		content string
		err     error
	}

	results := make(chan result, len(queries))
	sem := make(chan struct{}, ra.MaxWorkers)

	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{query: query, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			// Check context before searching.
			if ctx.Err() != nil {
				results <- result{query: query, err: ctx.Err()}
				return
			}

			content, err := searchFn(query)
			results <- result{query: query, content: content, err: err}
		}(q)
	}

	// Close results channel when all workers are done.
	go func() {
		wg.Wait()
		close(results)
	}()

	var findings []ResearchFinding
	for r := range results {
		if r.err != nil {
			continue
		}
		if r.content == "" {
			continue
		}
		findings = append(findings, ResearchFinding{
			Content:    r.content,
			Source:     r.query,
			Relevance:  0.5, // Default; will be updated by RankFindings.
			Confidence: 0.7,
		})
	}
	return findings
}

// RankFindings scores findings by relevance to the original query and returns them sorted.
func (ra *ResearchAgent) RankFindings(findings []ResearchFinding, query string) []ResearchFinding {
	if len(findings) == 0 {
		return nil
	}

	queryTerms := researchExtractKeywords(query)
	queryLower := strings.ToLower(query)

	for i := range findings {
		score := researchComputeRelevance(findings[i].Content, queryLower, queryTerms)
		findings[i].Relevance = score
		// Confidence is based on content length and term density.
		findings[i].Confidence = researchComputeConfidence(findings[i].Content, queryTerms)
	}

	// Sort by relevance descending.
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Relevance > findings[j].Relevance
	})

	return findings
}

// Synthesize combines findings into a coherent summary, deduplicating overlapping information.
func (ra *ResearchAgent) Synthesize(findings []ResearchFinding, query string) string {
	if len(findings) == 0 {
		return "No findings available for: " + query
	}

	// Deduplicate by checking for significant content overlap.
	seen := make([]string, 0, len(findings))
	unique := make([]ResearchFinding, 0, len(findings))
	for _, f := range findings {
		if isDuplicate(f.Content, seen) {
			continue
		}
		seen = append(seen, f.Content)
		unique = append(unique, f)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Research summary for: %s\n\n", query))

	for i, f := range unique {
		if i >= 10 {
			break // Limit to top 10 findings.
		}
		content := f.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] (relevance: %.2f, confidence: %.2f)\n   %s\n\n",
			i+1, f.Source, f.Relevance, f.Confidence, content))
	}

	return sb.String()
}

// FormatResult produces a human-readable formatted output of a ResearchResult.
func (ra *ResearchAgent) FormatResult(result *ResearchResult) string {
	if result == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Research: %q\n", result.Query))
	sb.WriteString(fmt.Sprintf("Duration: %.1fs | Sources: %d | Findings: %d\n",
		result.Duration.Seconds(), len(result.Sources), len(result.Findings)))
	sb.WriteString(strings.Repeat("─", 41))
	sb.WriteString("\n\n")

	// Key findings (top 5).
	sb.WriteString("Key findings:\n")
	limit := len(result.Findings)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		f := result.Findings[i]
		content := f.Content
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s (confidence: %.2f)\n", i+1, content, f.Confidence))
	}

	sb.WriteString("\nSummary: ")
	sb.WriteString(ra.Synthesize(result.Findings, result.Query))

	return sb.String()
}

// --- Helper functions ---

// researchExtractKeywords splits a question into meaningful terms, filtering stop words.
func researchExtractKeywords(text string) []string {
	stopWords := map[string]struct{}{
		"how": {}, "does": {}, "do": {}, "is": {}, "are": {},
		"the": {}, "a": {}, "an": {}, "in": {}, "on": {},
		"to": {}, "for": {}, "of": {}, "and": {}, "or": {},
		"this": {}, "that": {}, "it": {}, "what": {}, "where": {},
		"when": {}, "why": {}, "which": {}, "with": {}, "from": {},
		"work": {}, "works": {}, "project": {},
	}

	words := strings.Fields(strings.ToLower(text))
	keywords := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, "?.,!\"'()[]{}:")
		if _, stop := stopWords[w]; stop {
			continue
		}
		if len(w) > 1 {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// researchComputeRelevance scores how relevant content is to a query using term frequency.
func researchComputeRelevance(content, queryLower string, queryTerms []string) float64 {
	if len(queryTerms) == 0 {
		return 0.5
	}

	contentLower := strings.ToLower(content)
	matchCount := 0
	for _, term := range queryTerms {
		if strings.Contains(contentLower, term) {
			matchCount++
		}
	}

	// Base score from term overlap.
	score := float64(matchCount) / float64(len(queryTerms))

	// Bonus for exact phrase match.
	if strings.Contains(contentLower, queryLower) {
		score = math.Min(1.0, score+0.3)
	}

	return math.Min(1.0, score)
}

// researchComputeConfidence estimates confidence based on content quality signals.
func researchComputeConfidence(content string, queryTerms []string) float64 {
	if len(content) == 0 {
		return 0.0
	}

	// Longer, more detailed content gets higher confidence.
	lengthScore := math.Min(1.0, float64(len(content))/500.0)

	// More query terms matched = higher confidence.
	contentLower := strings.ToLower(content)
	matchCount := 0
	for _, term := range queryTerms {
		if strings.Contains(contentLower, term) {
			matchCount++
		}
	}
	var termScore float64
	if len(queryTerms) > 0 {
		termScore = float64(matchCount) / float64(len(queryTerms))
	}

	confidence := (lengthScore*0.4 + termScore*0.6)
	return math.Round(confidence*100) / 100
}

// isDuplicate checks if content has significant overlap with previously seen content.
func isDuplicate(content string, seen []string) bool {
	if len(seen) == 0 {
		return false
	}
	contentLower := strings.ToLower(content)
	for _, s := range seen {
		sLower := strings.ToLower(s)
		// Consider it a duplicate if either contains 80%+ of the other.
		shorter := contentLower
		longer := sLower
		if len(shorter) > len(longer) {
			shorter, longer = longer, shorter
		}
		if len(shorter) == 0 {
			continue
		}
		if strings.Contains(longer, shorter) {
			return true
		}
		// Check word-level overlap.
		shortWords := strings.Fields(shorter)
		if len(shortWords) == 0 {
			continue
		}
		overlap := 0
		for _, w := range shortWords {
			if strings.Contains(longer, w) {
				overlap++
			}
		}
		if float64(overlap)/float64(len(shortWords)) > 0.8 {
			return true
		}
	}
	return false
}
