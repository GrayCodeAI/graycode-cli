package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

// SelfReview implements "rubber duck debugging" -- asking the model to explain
// its generated code before applying it. Research (Self-Debugging, Chen et al.):
// +12% on MBPP by having the model explain its code to catch inconsistencies.
//
// This runs between code generation and file write, catching:
//   - Logic errors the model introduced
//   - Missing edge cases
//   - Misunderstanding of the original intent
//   - Potential regressions

// SelfReviewResult holds the outcome of an LLM self-review of a proposed change.
type SelfReviewResult struct {
	Approved    bool     // whether the change should be applied
	Issues      []string // problems found during review
	Suggestions []string // improvements that could be made
	Confidence  float64  // 0.0-1.0: how confident the reviewer is in its assessment
}

// ConfidenceThreshold is the minimum confidence score for auto-approval.
// Below this, the review will not approve even if no issues are found.
const ConfidenceThreshold = 0.7

// ReviewBeforeWrite asks the model to review its own proposed changes before
// they are applied to a file. It compares old and new content in the context
// of the user's original intent and returns approval or a list of issues.
//
// If confidence is below ConfidenceThreshold (0.7), the review will suggest
// fixes rather than approving, regardless of whether explicit issues were found.
func ReviewBeforeWrite(ctx context.Context, llm LLMClient, model string,
	intent string, filePath string, oldContent string, newContent string) (*SelfReviewResult, error) {

	if llm == nil {
		return nil, fmt.Errorf("self-review: no LLM client configured")
	}

	prompt := buildSelfReviewPrompt(intent, filePath, oldContent, newContent)
	resp, err := llm.Chat(ctx, buildReviewMessages(prompt), buildReviewOptions(model))
	if err != nil {
		return nil, fmt.Errorf("self-review: LLM call failed: %w", err)
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, fmt.Errorf("self-review: empty response from LLM")
	}

	result := parseSelfReview(resp.Content)

	// Enforce the confidence threshold: low confidence means we should not
	// auto-approve, even if the model said it looks fine.
	if result.Confidence < ConfidenceThreshold && len(result.Issues) == 0 {
		result.Approved = false
		result.Suggestions = append(result.Suggestions, "Low confidence score; manual review recommended before applying.")
	}

	return result, nil
}

// buildSelfReviewPrompt constructs the review prompt including the diff,
// original intent, and structured output format.
func buildSelfReviewPrompt(intent, filePath, oldContent, newContent string) string {
	var b strings.Builder

	b.WriteString("You are a meticulous code reviewer. Review the following proposed change BEFORE it is written to disk.\n\n")
	b.WriteString(fmt.Sprintf("ORIGINAL INTENT: %s\n\n", intent))
	b.WriteString(fmt.Sprintf("FILE: %s\n\n", filePath))

	b.WriteString("BEFORE (current content):\n```\n")
	b.WriteString(truncateForReview(oldContent, 3000))
	b.WriteString("\n```\n\n")

	b.WriteString("AFTER (proposed content):\n```\n")
	b.WriteString(truncateForReview(newContent, 3000))
	b.WriteString("\n```\n\n")

	b.WriteString(`Evaluate whether this change correctly implements the stated intent.

Check for:
1. Does the change match the intent? Are all requirements addressed?
2. Are there any logic errors or bugs introduced?
3. Are edge cases handled (nil checks, empty inputs, boundary conditions)?
4. Could this change cause regressions in existing functionality?
5. Is the code style consistent with the surrounding code?

Respond in exactly this format:

APPROVED: yes|no
CONFIDENCE: <number between 0.0 and 1.0>
ISSUES: <comma-separated list of issues, or "none">
SUGGESTIONS: <comma-separated list of improvements, or "none">`)

	return b.String()
}

// buildReviewMessages wraps the review prompt in a message slice.
func buildReviewMessages(prompt string) []client.EyrieMessage {
	return []client.EyrieMessage{
		{Role: "user", Content: prompt},
	}
}

// buildReviewOptions returns ChatOptions suitable for a self-review call.
func buildReviewOptions(model string) client.ChatOptions {
	return client.ChatOptions{
		Model:     model,
		MaxTokens: 512,
	}
}

// parseSelfReview extracts structured fields from the LLM's review response.
func parseSelfReview(response string) *SelfReviewResult {
	result := &SelfReviewResult{
		Confidence: 0.5, // default to uncertain
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "APPROVED:"):
			val := strings.TrimSpace(line[len("APPROVED:"):])
			result.Approved = strings.EqualFold(val, "yes")

		case strings.HasPrefix(upper, "CONFIDENCE:"):
			val := strings.TrimSpace(line[len("CONFIDENCE:"):])
			if conf, err := strconv.ParseFloat(val, 64); err == nil {
				if conf >= 0 && conf <= 1 {
					result.Confidence = conf
				}
			}

		case strings.HasPrefix(upper, "ISSUES:"):
			val := strings.TrimSpace(line[len("ISSUES:"):])
			if !strings.EqualFold(val, "none") && val != "" {
				for _, issue := range strings.Split(val, ",") {
					issue = strings.TrimSpace(issue)
					if issue != "" {
						result.Issues = append(result.Issues, issue)
					}
				}
			}

		case strings.HasPrefix(upper, "SUGGESTIONS:"):
			val := strings.TrimSpace(line[len("SUGGESTIONS:"):])
			if !strings.EqualFold(val, "none") && val != "" {
				for _, sug := range strings.Split(val, ",") {
					sug = strings.TrimSpace(sug)
					if sug != "" {
						result.Suggestions = append(result.Suggestions, sug)
					}
				}
			}
		}
	}

	// If there are issues, the result should not be approved regardless of
	// what the model said in the APPROVED field.
	if len(result.Issues) > 0 {
		result.Approved = false
	}

	return result
}

// truncateForReview truncates content for inclusion in a review prompt,
// keeping the beginning and end for context.
func truncateForReview(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	half := maxLen / 2
	return content[:half] + "\n... (truncated) ...\n" + content[len(content)-half:]
}
