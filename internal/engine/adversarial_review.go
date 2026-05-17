package engine

import "fmt"

// ReviewFinding is a single issue found during adversarial review.
type ReviewFinding struct {
	Severity string // HIGH, MEDIUM, LOW
	File     string
	Line     int
	Category string // edge-case, error-handling, security, performance, logic
	Issue    string
	Fix      string
}

// AdversarialReview holds the review configuration and results.
type AdversarialReview struct {
	Findings []ReviewFinding
}

// ReviewCategories are the areas the adversarial reviewer must check.
var ReviewCategories = []string{
	"edge-cases",     // nil, empty, overflow, concurrent access
	"error-handling", // unchecked errors, missing rollback, panic paths
	"security",       // injection, auth bypass, data exposure, secrets
	"performance",    // N+1, unbounded alloc, blocking calls, hot loops
	"logic",          // off-by-one, race conditions, deadlocks, wrong state
}

// ReviewPrompt generates the adversarial review system prompt.
func ReviewPrompt(files []string) string {
	return fmt.Sprintf(`You are an adversarial code reviewer. Your job is to FIND PROBLEMS.

RULES:
- You MUST find issues. Zero findings means you didn't look hard enough.
- No "looks good" allowed. Assume problems exist and find them.
- Be specific: file, line, what's wrong, how to fix it.
- Categorize each finding: %v

REVIEW THESE FILES:
%v

OUTPUT FORMAT (one per finding):
**SEVERITY** — file:line — category — description
  Fix: suggested fix

After all findings, rate overall quality: PASS (minor issues only) / CONCERNS (medium issues) / FAIL (critical issues)`,
		ReviewCategories, files)
}

// FormatFindings renders findings as a readable report.
func (ar *AdversarialReview) FormatFindings() string {
	if len(ar.Findings) == 0 {
		return "No findings (re-analyze required — adversarial review must find issues)."
	}
	var result string
	high, med, low := 0, 0, 0
	for _, f := range ar.Findings {
		result += fmt.Sprintf("**%s** — %s:%d — %s — %s\n", f.Severity, f.File, f.Line, f.Category, f.Issue)
		if f.Fix != "" {
			result += fmt.Sprintf("  Fix: %s\n", f.Fix)
		}
		switch f.Severity {
		case "HIGH":
			high++
		case "MEDIUM":
			med++
		default:
			low++
		}
	}
	result += fmt.Sprintf("\nSummary: %d HIGH, %d MEDIUM, %d LOW\n", high, med, low)
	if high > 0 {
		result += "Verdict: FAIL — critical issues must be addressed\n"
	} else if med > 2 {
		result += "Verdict: CONCERNS — review medium issues before merging\n"
	} else {
		result += "Verdict: PASS — minor issues only\n"
	}
	return result
}
