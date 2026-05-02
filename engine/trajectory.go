package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

// TrajectoryRun records a single attempt at completing a task.
type TrajectoryRun struct {
	ID       int
	Messages []client.EyrieMessage
	Success  bool
	Summary  string // distilled lessons from this run
	Tokens   int
}

// TrajectoryDistiller runs a task multiple times, summarizing what worked,
// and retrying with distilled knowledge from previous attempts.
type TrajectoryDistiller struct {
	maxRuns int // default 3
	session *Session
}

// NewTrajectoryDistiller creates a new distiller wrapping the given session.
func NewTrajectoryDistiller(session *Session, maxRuns int) *TrajectoryDistiller {
	if maxRuns <= 0 {
		maxRuns = 3
	}
	return &TrajectoryDistiller{
		maxRuns: maxRuns,
		session: session,
	}
}

// RunWithDistillation executes the prompt, and if it fails, retries with
// accumulated trajectory summaries from prior attempts. Returns the best result.
func (td *TrajectoryDistiller) RunWithDistillation(ctx context.Context, prompt string) (string, error) {
	var runs []TrajectoryRun

	for attempt := 0; attempt < td.maxRuns; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Build the augmented prompt with trajectory context from prior runs.
		augmented := prompt
		if len(runs) > 0 {
			augmented = buildAugmentedPrompt(prompt, runs)
		}

		// Snapshot current messages so we can restore after each attempt.
		savedMessages := make([]client.EyrieMessage, len(td.session.messages))
		copy(savedMessages, td.session.messages)

		// Add the user prompt.
		td.session.AddUser(augmented)

		// Collect the response by running the stream.
		ch, err := td.session.Stream(ctx)
		if err != nil {
			td.session.messages = savedMessages
			return "", fmt.Errorf("trajectory run %d: %w", attempt+1, err)
		}

		var response strings.Builder
		var tokens int
		hasError := false

		for ev := range ch {
			switch ev.Type {
			case "content":
				response.WriteString(ev.Content)
			case "error":
				hasError = true
				response.WriteString("[ERROR] " + ev.Content)
			case "usage":
				if ev.Usage != nil {
					tokens += ev.Usage.PromptTokens + ev.Usage.CompletionTokens
				}
			}
		}

		// Capture the messages generated during this run.
		runMessages := make([]client.EyrieMessage, len(td.session.messages))
		copy(runMessages, td.session.messages)

		run := TrajectoryRun{
			ID:       attempt + 1,
			Messages: runMessages,
			Success:  !hasError && response.Len() > 0,
			Summary:  SummarizeTrajectory(runMessages[len(savedMessages):]),
			Tokens:   tokens,
		}
		runs = append(runs, run)

		// If successful, return immediately.
		if run.Success {
			return response.String(), nil
		}

		// Restore messages for next attempt.
		td.session.messages = savedMessages
	}

	// All attempts failed; return the best one.
	best := td.BestRun(runs)
	if best == nil {
		return "", fmt.Errorf("all %d trajectory runs failed", td.maxRuns)
	}

	// Extract text content from the best run's messages.
	var result strings.Builder
	for _, msg := range best.Messages {
		if msg.Role == "assistant" && msg.Content != "" {
			result.WriteString(msg.Content)
		}
	}
	return result.String(), nil
}

// SummarizeTrajectory extracts a concise summary from a sequence of messages:
// what was attempted, what failed, key decisions made, and files touched.
func SummarizeTrajectory(messages []client.EyrieMessage) string {
	var b strings.Builder
	var attempted []string
	var failures []string
	var filesTouched []string

	seenFiles := make(map[string]bool)

	for _, msg := range messages {
		content := msg.Content

		// Track tool usage.
		for _, tc := range msg.ToolUse {
			attempted = append(attempted, tc.Name)
			// Extract file paths from tool arguments.
			for _, key := range []string{"path", "file", "file_path", "command"} {
				if v, ok := tc.Arguments[key]; ok {
					if s, ok := v.(string); ok && !seenFiles[s] {
						seenFiles[s] = true
						filesTouched = append(filesTouched, s)
					}
				}
			}
		}

		// Track errors.
		if msg.ToolResult != nil && msg.ToolResult.IsError {
			failures = append(failures, truncateStr(msg.ToolResult.Content, 100))
		}

		// Track error mentions in content.
		if strings.Contains(strings.ToLower(content), "error") ||
			strings.Contains(strings.ToLower(content), "failed") {
			failures = append(failures, truncateStr(content, 100))
		}
	}

	if len(attempted) > 0 {
		b.WriteString("Tools used: " + strings.Join(dedup(attempted), ", ") + ". ")
	}
	if len(filesTouched) > 0 {
		b.WriteString("Files touched: " + strings.Join(filesTouched, ", ") + ". ")
	}
	if len(failures) > 0 {
		// Limit to first 3 failures.
		if len(failures) > 3 {
			failures = failures[:3]
		}
		b.WriteString("Failures: " + strings.Join(failures, "; ") + ". ")
	}
	if b.Len() == 0 {
		return "No significant actions recorded."
	}
	return b.String()
}

// BestRun returns the run with Success=true (preferring earlier), or the
// highest-quality failure (most messages, fewest errors).
func (td *TrajectoryDistiller) BestRun(runs []TrajectoryRun) *TrajectoryRun {
	if len(runs) == 0 {
		return nil
	}

	// Prefer the first successful run.
	for i := range runs {
		if runs[i].Success {
			return &runs[i]
		}
	}

	// Among failures, pick the one with the most messages (most progress).
	best := &runs[0]
	for i := 1; i < len(runs); i++ {
		if len(runs[i].Messages) > len(best.Messages) {
			best = &runs[i]
		}
	}
	return best
}

// buildAugmentedPrompt prepends trajectory context to the original prompt.
func buildAugmentedPrompt(prompt string, runs []TrajectoryRun) string {
	var b strings.Builder
	b.WriteString("TRAJECTORY CONTEXT (previous attempts):\n")
	for _, run := range runs {
		status := "FAILED"
		if run.Success {
			status = "SUCCEEDED"
		}
		b.WriteString(fmt.Sprintf("  Attempt %d [%s]: %s\n", run.ID, status, run.Summary))
	}
	b.WriteString("\nUse the above lessons to avoid repeating mistakes.\n\n")
	b.WriteString(prompt)
	return b.String()
}

// dedup returns unique elements preserving order.
func dedup(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

// truncateStr truncates s to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
