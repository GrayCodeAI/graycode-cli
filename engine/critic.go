package engine

import (
	"fmt"
	"strings"
)

// PatchVerdict is the result of a critic's pre-screening of a patch.
type PatchVerdict struct {
	Likely     string   // "correct", "incorrect", "uncertain"
	Issues     []string // specific issues found
	Confidence float64  // 0-1
}

// Critic provides fast pre-validation of patches using a cheap model before
// expensive execution. It generates a prompt for the cheap model and parses
// the response into a structured verdict.
type Critic struct {
	model string // cheap/fast model for pre-screening
}

// NewCritic creates a new critic that will use the given model for screening.
func NewCritic(model string) *Critic {
	return &Critic{model: model}
}

// Model returns the model name used for pre-screening.
func (c *Critic) Model() string {
	return c.model
}

// PreScreenPatch asks the cheap model whether a patch looks correct given the
// stated intent. It builds a prompt, and returns a verdict. In this
// implementation, the caller is expected to send the prompt to the model and
// pass the response to ParseVerdict. This method constructs a PatchVerdict
// based on a simple heuristic comparison when no model call is available.
func (c *Critic) PreScreenPatch(originalContent, patchedContent, intent string) *PatchVerdict {
	// Quick heuristic checks when we can't call the model
	verdict := &PatchVerdict{
		Likely:     "uncertain",
		Confidence: 0.5,
	}

	if originalContent == patchedContent {
		verdict.Likely = "incorrect"
		verdict.Issues = append(verdict.Issues, "patch produces no changes")
		verdict.Confidence = 0.9
		return verdict
	}

	if strings.TrimSpace(patchedContent) == "" {
		verdict.Likely = "incorrect"
		verdict.Issues = append(verdict.Issues, "patch deletes all content")
		verdict.Confidence = 0.95
		return verdict
	}

	// Check for dramatically different size (potential data loss)
	origLen := len(originalContent)
	patchLen := len(patchedContent)
	if origLen > 0 && patchLen < origLen/4 {
		verdict.Likely = "incorrect"
		verdict.Issues = append(verdict.Issues, "patch removes more than 75% of content")
		verdict.Confidence = 0.8
		return verdict
	}

	return verdict
}

// BuildPrompt constructs a prompt for the cheap model to evaluate a patch.
func (c *Critic) BuildPrompt(original, patched, intent string) string {
	return fmt.Sprintf(`You are a code review critic. Evaluate whether this patch correctly implements the stated intent.

Intent: %s

Original code:
%s%s%s

Patched code:
%s%s%s

Respond with exactly one line in this format:
VERDICT: correct|incorrect|uncertain CONFIDENCE: 0.0-1.0
ISSUES: comma-separated list of issues (or "none")

Example:
VERDICT: correct CONFIDENCE: 0.9
ISSUES: none`,
		intent,
		"```\n", truncateForPrompt(original, 2000), "\n```",
		"```\n", truncateForPrompt(patched, 2000), "\n```")
}

// ParseVerdict parses a model response into a structured PatchVerdict.
func (c *Critic) ParseVerdict(response string) *PatchVerdict {
	verdict := &PatchVerdict{
		Likely:     "uncertain",
		Confidence: 0.5,
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse VERDICT line
		if strings.HasPrefix(strings.ToUpper(line), "VERDICT:") {
			rest := strings.TrimPrefix(line, "VERDICT:")
			rest = strings.TrimPrefix(rest, "verdict:")
			rest = strings.TrimPrefix(rest, "Verdict:")
			rest = strings.TrimSpace(rest)

			parts := strings.Fields(rest)
			if len(parts) >= 1 {
				v := strings.ToLower(parts[0])
				switch v {
				case "correct", "incorrect", "uncertain":
					verdict.Likely = v
				}
			}

			// Look for CONFIDENCE in same line
			if idx := strings.Index(strings.ToUpper(rest), "CONFIDENCE:"); idx >= 0 {
				confStr := strings.TrimSpace(rest[idx+len("CONFIDENCE:"):])
				confParts := strings.Fields(confStr)
				if len(confParts) >= 1 {
					var conf float64
					if _, err := fmt.Sscanf(confParts[0], "%f", &conf); err == nil {
						if conf >= 0 && conf <= 1 {
							verdict.Confidence = conf
						}
					}
				}
			}
		}

		// Parse ISSUES line
		if strings.HasPrefix(strings.ToUpper(line), "ISSUES:") {
			rest := line[len("ISSUES:"):]
			rest = strings.TrimSpace(rest)
			if strings.ToLower(rest) != "none" && rest != "" {
				issues := strings.Split(rest, ",")
				for _, issue := range issues {
					issue = strings.TrimSpace(issue)
					if issue != "" {
						verdict.Issues = append(verdict.Issues, issue)
					}
				}
			}
		}
	}

	return verdict
}

// ShouldBlock returns true if the verdict indicates the patch should be
// blocked (verdict is "incorrect" with confidence > 0.8).
func (c *Critic) ShouldBlock(verdict *PatchVerdict) bool {
	if verdict == nil {
		return false
	}
	return verdict.Likely == "incorrect" && verdict.Confidence > 0.8
}

// truncateForPrompt truncates content to fit within a prompt size limit,
// keeping the beginning and end for context.
func truncateForPrompt(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	half := maxLen / 2
	return content[:half] + "\n... (truncated) ...\n" + content[len(content)-half:]
}
