package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Issue represents a project issue (bug, feature request, etc.) for similarity search.
type Issue struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Labels     []string   `json:"labels"`
	State      string     `json:"state"`      // "open", "closed"
	Resolution string     `json:"resolution"` // how the issue was resolved
	CreatedAt  time.Time  `json:"created_at"`
	ClosedAt   *time.Time `json:"closed_at"`
	Tokens     []string   `json:"tokens"` // tokenized title+body for search
}

// SimilarIssue represents a search result with similarity scoring.
type SimilarIssue struct {
	Issue         *Issue   `json:"issue"`
	Score         float64  `json:"score"`
	MatchingTerms []string `json:"matching_terms"`
}

// IssueIndex provides BM25-based similarity search over a collection of issues.
type IssueIndex struct {
	Issues        []*Issue         `json:"issues"`
	InvertedIndex map[string][]int `json:"inverted_index"`
	mu            sync.RWMutex
}

// BM25 tuning parameters.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// stopWords contains common English stop words to filter during tokenization.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "it": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "and": true, "or": true, "not": true, "with": true,
	"as": true, "by": true, "from": true, "that": true, "this": true,
	"be": true, "are": true, "was": true, "were": true, "been": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "but": true, "if": true,
	"when": true, "where": true, "how": true, "what": true, "which": true,
	"who": true, "whom": true, "all": true, "each": true, "every": true,
	"i": true, "we": true, "you": true, "he": true, "she": true,
	"they": true, "me": true, "us": true, "him": true, "her": true,
	"them": true, "my": true, "our": true, "your": true, "his": true,
	"its": true, "their": true, "am": true, "so": true, "no": true,
}

// NewIssueIndex creates a new empty IssueIndex ready for use.
func NewIssueIndex() *IssueIndex {
	return &IssueIndex{
		Issues:        make([]*Issue, 0),
		InvertedIndex: make(map[string][]int),
	}
}

// issueTokenize splits text into normalized, de-duplicated tokens suitable for search.
func issueTokenize(text string) []string {
	text = strings.ToLower(text)
	// Split on non-alphanumeric characters
	splitter := func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	words := strings.FieldsFunc(text, splitter)

	seen := make(map[string]bool)
	var tokens []string
	for _, w := range words {
		if len(w) < 2 {
			continue
		}
		if stopWords[w] {
			continue
		}
		if !seen[w] {
			seen[w] = true
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// AddIssue adds an issue to the index, tokenizing its title and body for search.
func (idx *IssueIndex) AddIssue(issue *Issue) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Tokenize title + body
	combined := issue.Title + " " + issue.Body
	issue.Tokens = issueTokenize(combined)

	docIndex := len(idx.Issues)
	idx.Issues = append(idx.Issues, issue)

	// Update inverted index
	for _, token := range issue.Tokens {
		idx.InvertedIndex[token] = append(idx.InvertedIndex[token], docIndex)
	}
}

// avgDocLength returns the average document length across all indexed issues.
func (idx *IssueIndex) avgDocLength() float64 {
	if len(idx.Issues) == 0 {
		return 0
	}
	total := 0
	for _, issue := range idx.Issues {
		total += len(issue.Tokens)
	}
	return float64(total) / float64(len(idx.Issues))
}

// termFrequency returns how many times a term appears in a document's tokens.
func termFrequency(term string, tokens []string) int {
	count := 0
	for _, t := range tokens {
		if t == term {
			count++
		}
	}
	return count
}

// FindSimilar searches for issues similar to the given query using BM25 scoring.
// It returns up to limit results sorted by relevance score.
func (idx *IssueIndex) FindSimilar(query string, limit int) []*SimilarIssue {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.Issues) == 0 {
		return nil
	}

	queryTokens := issueTokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	avgDL := idx.avgDocLength()
	n := float64(len(idx.Issues))

	type scored struct {
		index         int
		score         float64
		matchingTerms []string
	}

	scores := make([]scored, len(idx.Issues))
	for i := range scores {
		scores[i].index = i
	}

	// BM25 scoring
	for _, term := range queryTokens {
		docFreq := len(idx.InvertedIndex[term])
		if docFreq == 0 {
			continue
		}

		// IDF component: log((N - df + 0.5) / (df + 0.5) + 1)
		idf := math.Log((n-float64(docFreq)+0.5)/(float64(docFreq)+0.5) + 1.0)

		for _, docIdx := range idx.InvertedIndex[term] {
			tf := float64(termFrequency(term, idx.Issues[docIdx].Tokens))
			dl := float64(len(idx.Issues[docIdx].Tokens))

			// BM25 term score
			numerator := tf * (bm25K1 + 1.0)
			denominator := tf + bm25K1*(1.0-bm25B+bm25B*(dl/avgDL))
			termScore := idf * (numerator / denominator)

			scores[docIdx].score += termScore
			scores[docIdx].matchingTerms = append(scores[docIdx].matchingTerms, term)
		}
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Collect top results with positive scores
	var results []*SimilarIssue
	for i := 0; i < len(scores) && len(results) < limit; i++ {
		if scores[i].score <= 0 {
			break
		}
		results = append(results, &SimilarIssue{
			Issue:         idx.Issues[scores[i].index],
			Score:         scores[i].score,
			MatchingTerms: dedupStrings(scores[i].matchingTerms),
		})
	}

	return results
}

// dedupStrings removes duplicate strings from a slice.
func dedupStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ghIssueJSON represents the JSON structure returned by `gh issue list --json`.
type ghIssueJSON struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Labels    []ghLabel  `json:"labels"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"createdAt"`
	ClosedAt  *time.Time `json:"closedAt"`
}

type ghLabel struct {
	Name string `json:"name"`
}

// ImportFromGitHub imports issues from GitHub using the gh CLI tool.
// It requires the gh CLI to be installed and authenticated.
func (idx *IssueIndex) ImportFromGitHub(projectDir string) error {
	// Check if gh is available
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}

	cmd := exec.Command(
		"gh", "issue", "list",
		"--state", "all",
		"--limit", "200",
		"--json", "number,title,body,labels,state,createdAt,closedAt",
	)
	cmd.Dir = projectDir

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gh issue list failed: %w", err)
	}

	var ghIssues []ghIssueJSON
	if err := json.Unmarshal(output, &ghIssues); err != nil {
		return fmt.Errorf("parsing gh output: %w", err)
	}

	for _, gi := range ghIssues {
		labels := make([]string, 0, len(gi.Labels))
		for _, l := range gi.Labels {
			labels = append(labels, l.Name)
		}

		state := strings.ToLower(gi.State)
		issue := &Issue{
			ID:        fmt.Sprintf("%d", gi.Number),
			Title:     gi.Title,
			Body:      gi.Body,
			Labels:    labels,
			State:     state,
			CreatedAt: gi.CreatedAt,
			ClosedAt:  gi.ClosedAt,
		}
		idx.AddIssue(issue)
	}

	return nil
}

// commitFixRef matches patterns like "fixes #N", "closes #N", "resolves #N" in commit messages.
var commitFixRef = regexp.MustCompile(`(?i)(?:fix(?:es|ed)?|clos(?:es|ed)?|resolv(?:es|ed)?)\s+#(\d+)`)

// ImportFromCommits extracts issue resolution info from git commit history.
// It parses "fixes #N" style references and maps them to commit messages.
func (idx *IssueIndex) ImportFromCommits(projectDir string) error {
	cmd := exec.Command("git", "log", "--oneline", "--all", "-500")
	cmd.Dir = projectDir

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git log failed: %w", err)
	}

	// Build a map from issue number to resolution description
	resolutions := make(map[string]string)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := commitFixRef.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				issueNum := match[1]
				// Use the commit message (minus the hash) as resolution
				parts := strings.SplitN(line, " ", 2)
				if len(parts) == 2 {
					resolutions[issueNum] = parts[1]
				}
			}
		}
	}

	// Apply resolutions to existing issues in the index
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, issue := range idx.Issues {
		if res, ok := resolutions[issue.ID]; ok {
			if issue.Resolution == "" {
				issue.Resolution = res
			}
		}
	}

	return nil
}

// SuggestResolution generates a resolution suggestion based on similar closed issues.
func SuggestResolution(similar []*SimilarIssue) string {
	if len(similar) == 0 {
		return "No similar issues found to suggest a resolution."
	}

	var suggestions []string
	for _, s := range similar {
		if s.Issue.State == "closed" && s.Issue.Resolution != "" {
			suggestions = append(suggestions, fmt.Sprintf(
				"Similar issue #%s was fixed by: %s",
				s.Issue.ID, s.Issue.Resolution,
			))
		}
	}

	if len(suggestions) == 0 {
		return "No resolved similar issues found to suggest a resolution."
	}

	return strings.Join(suggestions, "\n")
}

// FormatIssueResults produces a human-readable summary of similar issue search results.
func FormatIssueResults(similar []*SimilarIssue) string {
	if len(similar) == 0 {
		return "No similar issues found."
	}

	var sb strings.Builder
	sb.WriteString("Similar Issues Found:\n")
	sb.WriteString(strings.Repeat("─", 21) + "\n")

	for i, s := range similar {
		// Convert score to percentage (capped at 99%)
		pct := int(math.Min(s.Score*100.0/maxScore(similar), 99))
		if pct < 1 {
			pct = 1
		}

		stateLabel := strings.ToUpper(s.Issue.State)
		sb.WriteString(fmt.Sprintf("%d. #%s %q (%d%% match, %s)\n",
			i+1, s.Issue.ID, s.Issue.Title, pct, stateLabel))

		if s.Issue.State == "closed" && s.Issue.Resolution != "" {
			sb.WriteString(fmt.Sprintf("   Resolution: %s\n", s.Issue.Resolution))
		} else if s.Issue.State == "open" {
			sb.WriteString("   No resolution yet\n")
		}

		if len(s.MatchingTerms) > 0 {
			quoted := make([]string, len(s.MatchingTerms))
			for j, t := range s.MatchingTerms {
				quoted[j] = fmt.Sprintf("%q", t)
			}
			sb.WriteString(fmt.Sprintf("   Matching: %s\n", strings.Join(quoted, ", ")))
		}

		if i < len(similar)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// maxScore returns the maximum score from a list of similar issues (used for percentage normalization).
func maxScore(similar []*SimilarIssue) float64 {
	if len(similar) == 0 {
		return 1.0
	}
	max := similar[0].Score
	for _, s := range similar[1:] {
		if s.Score > max {
			max = s.Score
		}
	}
	if max <= 0 {
		return 1.0
	}
	return max
}

// BuildSearchContext formats similar issues as context for agent injection.
// This is suitable for including in LLM prompts to provide relevant historical context.
func BuildSearchContext(similar []*SimilarIssue) string {
	if len(similar) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Related Issues Context\n\n")
	sb.WriteString("The following past issues are similar to the current task:\n\n")

	for _, s := range similar {
		sb.WriteString(fmt.Sprintf("### Issue #%s: %s [%s]\n", s.Issue.ID, s.Issue.Title, strings.ToUpper(s.Issue.State)))

		if s.Issue.Body != "" {
			// Truncate long bodies for context
			body := s.Issue.Body
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("Description: %s\n", body))
		}

		if len(s.Issue.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(s.Issue.Labels, ", ")))
		}

		if s.Issue.Resolution != "" {
			sb.WriteString(fmt.Sprintf("Resolution: %s\n", s.Issue.Resolution))
		}

		sb.WriteString(fmt.Sprintf("Relevance: %.0f%% match on terms: %s\n",
			math.Min(s.Score*100.0/maxScore(similar), 99),
			strings.Join(s.MatchingTerms, ", ")))
		sb.WriteString("\n")
	}

	sb.WriteString("Use these past issues to inform your approach. ")
	sb.WriteString("If a similar issue was already resolved, consider applying the same fix pattern.\n")

	return sb.String()
}
