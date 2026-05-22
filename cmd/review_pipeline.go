package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ReviewChatFn is a function that sends a prompt to an LLM and returns the response.
type ReviewChatFn func(ctx context.Context, prompt string) (string, error)

// ReviewConcern represents one aspect of code review.
type ReviewConcern struct {
	Name   string // "security", "performance", "style", "bugs", "correctness"
	Prompt string // specialized review prompt for this concern
}

// ReviewFinding represents one issue found.
type ReviewFinding struct {
	Concern  string
	Severity string // "critical", "high", "medium", "low"
	File     string
	Line     int
	Message  string
	Fix      string // suggested fix
}

// DefaultConcerns returns standard review concerns.
func DefaultConcerns() []ReviewConcern {
	return []ReviewConcern{
		{
			Name: "security",
			Prompt: `Review the following code for security vulnerabilities:
- Injection attacks (SQL, command, path traversal)
- Authentication and authorization flaws
- Sensitive data exposure
- Insecure deserialization
- Missing input validation
Report each issue with: File, Line (approx), Severity (critical/high/medium/low), Description, Fix.`,
		},
		{
			Name: "performance",
			Prompt: `Review the following code for performance issues:
- Unnecessary allocations and copies
- O(n^2) or worse algorithms where O(n) is possible
- Missing caching opportunities
- Unbounded growth (slices, maps, channels)
- Blocking operations that could be async
Report each issue with: File, Line (approx), Severity (critical/high/medium/low), Description, Fix.`,
		},
		{
			Name: "bugs",
			Prompt: `Review the following code for bugs and logic errors:
- Off-by-one errors
- Nil/null pointer dereferences
- Race conditions
- Resource leaks (unclosed files, connections)
- Incorrect error handling
- Dead code and unreachable branches
Report each issue with: File, Line (approx), Severity (critical/high/medium/low), Description, Fix.`,
		},
		{
			Name: "correctness",
			Prompt: `Review the following code for correctness:
- Does the code match its stated intent (function names, comments)?
- Are edge cases handled?
- Are return values and errors checked?
- Is concurrency handled correctly (mutexes, channels)?
- Are API contracts respected?
Report each issue with: File, Line (approx), Severity (critical/high/medium/low), Description, Fix.`,
		},
		{
			Name: "style",
			Prompt: `Review the following code for style and maintainability:
- Naming conventions
- Function length and complexity
- Code duplication
- Missing or misleading comments
- Consistent formatting
Report each issue with: File, Line (approx), Severity (critical/high/medium/low), Description, Fix.`,
		},
	}
}

// RunReviewPipeline performs multi-concern parallel review using the provided
// chat function to query an LLM. Returns deduplicated findings sorted by
// severity and a formatted report string.
func RunReviewPipeline(ctx context.Context, files []string, concerns []ReviewConcern, chatFn ReviewChatFn) ([]ReviewFinding, string) {
	if len(files) == 0 || len(concerns) == 0 {
		return nil, "No files or concerns specified."
	}
	if chatFn == nil {
		return nil, "No LLM chat function provided."
	}

	var mu sync.Mutex
	var allFindings []ReviewFinding
	var wg sync.WaitGroup

	for _, concern := range concerns {
		wg.Add(1)
		go func(c ReviewConcern) {
			defer wg.Done()
			findings := reviewForConcern(ctx, files, c, chatFn)
			mu.Lock()
			allFindings = append(allFindings, findings...)
			mu.Unlock()
		}(concern)
	}
	wg.Wait()

	allFindings = deduplicateFindings(allFindings)
	sortBySeverity(allFindings)

	report := FormatReviewReport(allFindings)
	return allFindings, report
}

// reviewForConcern builds a review prompt for a single concern, sends it to
// the LLM via chatFn, and parses the response into structured findings.
func reviewForConcern(ctx context.Context, files []string, concern ReviewConcern, chatFn ReviewChatFn) []ReviewFinding {
	prompt := buildReviewPrompt(files, concern)

	response, err := chatFn(ctx, prompt)
	if err != nil {
		return nil
	}

	return parseReviewFindings(response, concern.Name)
}

// llmFinding mirrors the JSON structure the LLM is asked to return.
type llmFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

// parseReviewFindings extracts structured findings from an LLM response.
// It looks for a JSON array in the response; if parsing fails, it returns a
// single finding containing the raw response so the user still sees output.
func parseReviewFindings(response string, concernName string) []ReviewFinding {
	// Try to find a JSON array in the response.
	jsonStart := strings.Index(response, "[")
	jsonEnd := strings.LastIndex(response, "]")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		var raw []llmFinding
		if err := json.Unmarshal([]byte(response[jsonStart:jsonEnd+1]), &raw); err == nil {
			findings := make([]ReviewFinding, 0, len(raw))
			for _, r := range raw {
				findings = append(findings, ReviewFinding{
					Concern:  concernName,
					Severity: r.Severity,
					File:     r.File,
					Line:     r.Line,
					Message:  r.Message,
					Fix:      r.Fix,
				})
			}
			return findings
		}
	}

	// Fallback: return the raw response as a single finding.
	trimmed := strings.TrimSpace(response)
	if trimmed == "" || strings.EqualFold(trimmed, "no issues found") || strings.EqualFold(trimmed, "no issues found.") {
		return nil
	}
	return []ReviewFinding{{
		Concern:  concernName,
		Severity: "medium",
		Message:  trimmed,
	}}
}

// buildReviewPrompt constructs the full prompt for a single review concern.
func buildReviewPrompt(files []string, concern ReviewConcern) string {
	var b strings.Builder
	b.WriteString(concern.Prompt)
	b.WriteString("\n\n## Output format\n\n")
	b.WriteString("Return your findings as a JSON array. Each element must have:\n")
	b.WriteString("  \"file\" (string), \"line\" (int), \"severity\" (critical|high|medium|low),\n")
	b.WriteString("  \"message\" (string), \"fix\" (string, suggested fix).\n")
	b.WriteString("If no issues are found, return an empty array: []\n")
	b.WriteString("\n## Files to review\n\n")
	for _, f := range files {
		b.WriteString(fmt.Sprintf("- %s\n", f))
	}
	return b.String()
}

// FormatReviewReport formats findings as a readable report grouped by severity.
func FormatReviewReport(findings []ReviewFinding) string {
	if len(findings) == 0 {
		return "No issues found."
	}

	var b strings.Builder
	b.WriteString("=== Review Report ===\n\n")

	// Group by severity
	groups := map[string][]ReviewFinding{}
	order := []string{"critical", "high", "medium", "low"}
	for _, f := range findings {
		groups[f.Severity] = append(groups[f.Severity], f)
	}

	for _, sev := range order {
		items := groups[sev]
		if len(items) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s (%d)\n\n", strings.ToUpper(sev), len(items)))
		for _, f := range items {
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			b.WriteString(fmt.Sprintf("  [%s] %s\n", loc, f.Message))
			if f.Fix != "" {
				b.WriteString(fmt.Sprintf("    Fix: %s\n", f.Fix))
			}
			b.WriteString("\n")
		}
	}

	// Summary line
	b.WriteString(fmt.Sprintf("--- %d issue(s) total ---\n", len(findings)))
	return b.String()
}

// deduplicateFindings removes findings with the same file, line, and message.
func deduplicateFindings(findings []ReviewFinding) []ReviewFinding {
	type key struct {
		file    string
		line    int
		message string
	}
	seen := map[key]bool{}
	var result []ReviewFinding
	for _, f := range findings {
		k := key{file: f.File, line: f.Line, message: f.Message}
		if seen[k] {
			continue
		}
		seen[k] = true
		result = append(result, f)
	}
	return result
}

// sortBySeverity sorts findings by severity (critical > high > medium > low).
func sortBySeverity(findings []ReviewFinding) {
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sort.Slice(findings, func(i, j int) bool {
		si := sevOrder[findings[i].Severity]
		sj := sevOrder[findings[j].Severity]
		if si != sj {
			return si < sj
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}
