package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// LLMClient is the minimal interface for calling an LLM from engine components.
type LLMClient interface {
	Chat(ctx context.Context, msgs []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error)
}

// Reflector provides verbal self-reflection on agent actions.
type Reflector struct {
	llm     LLMClient
	model   string
	mu      sync.Mutex
	history []ReflectionEntry
}

// ReflectionEntry records a single reflection.
type ReflectionEntry struct {
	Attempt    int
	TaskGoal   string
	WhatFailed string
	WhyFailed  string
	WhatToDo   string
	Timestamp  time.Time
}

// NewReflector creates a reflector with the given LLM client.
func NewReflector(llm LLMClient, model string) *Reflector {
	return &Reflector{llm: llm, model: model}
}

// History returns all reflection entries.
func (r *Reflector) History() []ReflectionEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ReflectionEntry{}, r.history...)
}

// Reflect analyzes a failure and records a lesson.
func (r *Reflector) Reflect(ctx context.Context, goal string, msgs []types.EyrieMessage, errorContext string) (*ReflectionEntry, error) {
	if r.llm == nil {
		return nil, fmt.Errorf("reflector: no LLM client configured")
	}
	r.mu.Lock()
	attempt := len(r.history) + 1
	r.mu.Unlock()

	prompt := buildReflectionPrompt(goal, msgs, errorContext)
	allMsgs := []types.EyrieMessage{{Role: "user", Content: prompt}}
	resp, err := r.llm.Chat(ctx, allMsgs, types.ChatOptions{Model: r.model, MaxTokens: 512})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Content == "" {
		return nil, fmt.Errorf("reflector: empty response from LLM")
	}
	entry := parseReflectionEntry(resp.Content, attempt, goal)
	entry.Timestamp = time.Now()
	r.mu.Lock()
	// Re-compute attempt number under lock to avoid duplicate numbering.
	entry.Attempt = len(r.history) + 1
	r.history = append(r.history, entry)
	r.mu.Unlock()
	return &entry, nil
}

func parseReflectionEntry(content string, attempt int, goal string) ReflectionEntry {
	entry := ReflectionEntry{Attempt: attempt, TaskGoal: goal}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "WHAT_FAILED:") || strings.HasPrefix(upper, "WHAT FAILED:") {
			entry.WhatFailed = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		} else if strings.HasPrefix(upper, "WHY_FAILED:") || strings.HasPrefix(upper, "WHY FAILED:") {
			entry.WhyFailed = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		} else if strings.HasPrefix(upper, "WHAT_TO_DO:") || strings.HasPrefix(upper, "WHAT TO DO:") {
			entry.WhatToDo = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		}
	}
	// Fallback for empty fields
	if entry.WhatFailed == "" {
		entry.WhatFailed = "(unable to parse reflection)"
	}
	if entry.WhyFailed == "" {
		entry.WhyFailed = "(unable to determine cause)"
	}
	if entry.WhatToDo == "" {
		entry.WhatToDo = "(no action determined)"
	}
	return entry
}

// buildReflectionPrompt constructs the reflection prompt from goal, messages, and error.
func buildReflectionPrompt(goal string, msgs []types.EyrieMessage, errorContext string) string {
	var sb strings.Builder
	sb.WriteString("TASK GOAL: " + goal + "\n\n")
	sb.WriteString("CONVERSATION TRANSCRIPT:\n")
	for _, m := range msgs {
		if m.ToolResult != nil {
			prefix := "[tool_result]"
			if m.ToolResult.IsError {
				prefix = "[tool_result ERROR]"
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", prefix, m.ToolResult.Content))
		} else if len(m.ToolUse) > 0 {
			for _, tu := range m.ToolUse {
				sb.WriteString(fmt.Sprintf("[%s] tool_call: %s\n", m.Role, tu.Name))
			}
		} else {
			sb.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, m.Content))
		}
	}
	sb.WriteString("\nFINAL ERROR: " + errorContext + "\n\n")
	sb.WriteString("Analyze this failure. Respond with exactly:\nWHAT_FAILED: <what went wrong>\nWHY_FAILED: <root cause>\nWHAT_TO_DO: <corrective action>")
	return sb.String()
}

// InjectReflections formats reflection history as a string for system prompt injection.
func (r *Reflector) InjectReflections() string {
	r.mu.Lock()
	history := r.history
	r.mu.Unlock()
	if len(history) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("REFLECTIONS FROM PREVIOUS ATTEMPTS:\n")
	for _, e := range history {
		sb.WriteString(fmt.Sprintf("Attempt %d (goal: %s):\n", e.Attempt, e.TaskGoal))
		sb.WriteString("  WHAT_FAILED: " + e.WhatFailed + "\n")
		sb.WriteString("  WHY_FAILED: " + e.WhyFailed + "\n")
		sb.WriteString("  WHAT_TO_DO: " + e.WhatToDo + "\n\n")
	}
	return sb.String()
}

// Reset clears all reflection history.
func (r *Reflector) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = nil
}

// parseReflection is an alias for backward compatibility with tests.
func parseReflection(content string) ReflectionEntry {
	return parseReflectionEntry(content, 0, "")
}
