package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
)

// Reflector generates verbal self-reflections after task attempts.
// Based on Reflexion (Shinn et al., NeurIPS 2023): verbal reinforcement
// learning achieves 91% on HumanEval by storing natural-language reflections
// in an episodic memory buffer for subsequent attempts.
//
// Instead of mechanical extraction ("tools used: X, files touched: Y"),
// we ask the LLM: "Analyze what went wrong and what should be done differently."
// This produces richer, actionable feedback that subsequent attempts can use.
type Reflector struct {
	client  LLMClient
	model   string
	history []Reflection
}

// LLMClient is the interface for sending chat requests to an LLM provider.
// It is satisfied by *client.EyrieClient.
type LLMClient interface {
	Chat(ctx context.Context, msgs []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error)
}

// Reflection captures a structured verbal self-reflection on a failed attempt.
type Reflection struct {
	Attempt    int       // which attempt number this reflects on
	TaskGoal   string    // what the task was trying to accomplish
	WhatFailed string    // what specifically went wrong
	WhyFailed  string    // root cause analysis
	WhatToDo   string    // what should be done differently next time
	Timestamp  time.Time // when the reflection was generated
}

// NewReflector creates a Reflector that uses the given LLM client and model
// to generate verbal reflections on failed task attempts.
func NewReflector(llm LLMClient, model string) *Reflector {
	return &Reflector{
		client: llm,
		model:  model,
	}
}

// Reflect generates a verbal self-reflection on a failed attempt by asking
// the LLM to analyze the conversation history and error. The resulting
// Reflection is appended to the internal history for later injection.
func (r *Reflector) Reflect(ctx context.Context, goal string, messages []client.EyrieMessage, errorMsg string) (*Reflection, error) {
	if r.client == nil {
		return nil, fmt.Errorf("reflector: no LLM client configured")
	}

	prompt := buildReflectionPrompt(goal, messages, errorMsg)
	resp, err := r.client.Chat(ctx, []client.EyrieMessage{
		{Role: "user", Content: prompt},
	}, client.ChatOptions{
		Model:     r.model,
		MaxTokens: 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("reflector: LLM call failed: %w", err)
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, fmt.Errorf("reflector: empty response from LLM")
	}

	ref := parseReflection(resp.Content)
	ref.Attempt = len(r.history) + 1
	ref.TaskGoal = goal
	ref.Timestamp = time.Now()

	r.history = append(r.history, *ref)
	return ref, nil
}

// InjectReflections formats all accumulated reflections into a block of text
// suitable for prepending to the next attempt's prompt. If there are no
// reflections yet, it returns an empty string.
func (r *Reflector) InjectReflections() string {
	if len(r.history) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("REFLECTIONS FROM PREVIOUS ATTEMPTS:\n")
	b.WriteString("Use these lessons to avoid repeating the same mistakes.\n\n")

	for _, ref := range r.history {
		b.WriteString(fmt.Sprintf("--- Attempt %d ---\n", ref.Attempt))
		b.WriteString(fmt.Sprintf("Goal: %s\n", ref.TaskGoal))
		b.WriteString(fmt.Sprintf("What failed: %s\n", ref.WhatFailed))
		b.WriteString(fmt.Sprintf("Why it failed: %s\n", ref.WhyFailed))
		b.WriteString(fmt.Sprintf("What to do differently: %s\n\n", ref.WhatToDo))
	}

	return b.String()
}

// History returns a copy of all reflections generated so far.
func (r *Reflector) History() []Reflection {
	out := make([]Reflection, len(r.history))
	copy(out, r.history)
	return out
}

// Reset clears the reflection history.
func (r *Reflector) Reset() {
	r.history = nil
}

// buildReflectionPrompt constructs the prompt sent to the LLM for reflection.
// It includes the task goal, a condensed conversation transcript, and the error.
func buildReflectionPrompt(goal string, messages []client.EyrieMessage, errorMsg string) string {
	var b strings.Builder
	b.WriteString("You are a reflective reasoning agent. A task attempt has failed and you must analyze what went wrong.\n\n")
	b.WriteString(fmt.Sprintf("TASK GOAL: %s\n\n", goal))

	// Include a condensed conversation transcript.
	b.WriteString("CONVERSATION TRANSCRIPT (condensed):\n")
	for _, msg := range messages {
		content := msg.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		if content != "" {
			b.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, content))
		}
		for _, tc := range msg.ToolUse {
			b.WriteString(fmt.Sprintf("[tool_call]: %s\n", tc.Name))
		}
		if msg.ToolResult != nil {
			status := "ok"
			if msg.ToolResult.IsError {
				status = "ERROR"
			}
			result := msg.ToolResult.Content
			if len(result) > 200 {
				result = result[:200] + "..."
			}
			b.WriteString(fmt.Sprintf("[tool_result %s]: %s\n", status, result))
		}
	}

	b.WriteString(fmt.Sprintf("\nFINAL ERROR: %s\n\n", errorMsg))

	b.WriteString(`Provide your reflection in exactly this format:

WHAT_FAILED: <one or two sentences describing what specifically went wrong>
WHY_FAILED: <root cause analysis -- why did the failure happen?>
WHAT_TO_DO: <concrete, actionable advice for the next attempt>

Be specific and actionable. Do not repeat generic advice.`)

	return b.String()
}

// parseReflection extracts structured fields from the LLM's reflection response.
func parseReflection(response string) *Reflection {
	ref := &Reflection{}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "WHAT_FAILED:"):
			ref.WhatFailed = strings.TrimSpace(line[len("WHAT_FAILED:"):])
		case strings.HasPrefix(upper, "WHY_FAILED:"):
			ref.WhyFailed = strings.TrimSpace(line[len("WHY_FAILED:"):])
		case strings.HasPrefix(upper, "WHAT_TO_DO:"):
			ref.WhatToDo = strings.TrimSpace(line[len("WHAT_TO_DO:"):])
		}
	}

	// If structured parsing failed, use the entire response as WhatFailed.
	if ref.WhatFailed == "" && ref.WhyFailed == "" && ref.WhatToDo == "" {
		ref.WhatFailed = truncateStr(strings.TrimSpace(response), 500)
		ref.WhyFailed = "Could not determine root cause from reflection."
		ref.WhatToDo = "Review the error message and conversation transcript carefully."
	}

	return ref
}
