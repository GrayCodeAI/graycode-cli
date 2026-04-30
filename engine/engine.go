package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hawk/eyrie/client"

	"github.com/GrayCodeAI/hawk/tool"
)

const (
	maxContextMessages = 100 // auto-compact threshold
	maxRecoveryRetries = 3  // max_tokens recovery attempts
)

// Session manages a conversation with an LLM via eyrie.
type Session struct {
	client   *client.EyrieClient
	registry *tool.Registry
	messages []client.EyrieMessage
	provider string
	model    string
	system   string
	Cost     Cost
}

// NewSession creates a new conversation session.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	detected := provider
	if detected == "" {
		detected = client.DetectProvider()
	}
	if model == "" {
		model = client.ResolveDefaultModel(detected)
	}
	return &Session{
		client:   client.NewEyrieClient(&client.EyrieConfig{Provider: detected}),
		registry: registry,
		provider: detected,
		model:    model,
		system:   systemPrompt,
	}
}

func (s *Session) Model() string    { return s.model }
func (s *Session) Provider() string { return s.provider }

func (s *Session) AddUser(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: content})
}

func (s *Session) AddAssistant(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: content})
}

func (s *Session) LoadMessages(msgs []client.EyrieMessage) {
	s.messages = msgs
}

func (s *Session) MessageCount() int { return len(s.messages) }

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type     string // content, thinking, tool_use, tool_result, done, error
	Content  string
	ToolName string
	ToolID   string
}

// Stream runs the agentic loop: LLM → tool_use → execute → loop.
func (s *Session) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 64)
	go s.agentLoop(ctx, ch)
	return ch, nil
}

func (s *Session) agentLoop(ctx context.Context, ch chan<- StreamEvent) {
	defer close(ch)

	recoveryCount := 0

	for {
		// Auto-compact if conversation is too long
		if len(s.messages) > maxContextMessages {
			s.compact()
		}

		opts := client.ChatOptions{
			Provider:  s.provider,
			Model:     s.model,
			MaxTokens: 16384,
			System:    s.system,
		}
		if s.registry != nil {
			opts.Tools = s.registry.EyrieTools()
		}

		result, err := s.client.StreamChat(ctx, s.messages, opts)
		if err != nil {
			// Handle prompt too long
			if strings.Contains(err.Error(), "too long") || strings.Contains(err.Error(), "too many tokens") {
				s.compact()
				result, err = s.client.StreamChat(ctx, s.messages, opts)
				if err != nil {
					ch <- StreamEvent{Type: "error", Content: err.Error()}
					return
				}
			} else {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
		}

		var textContent string
		var toolCalls []client.ToolCall
		var stopReason string

		for ev := range result.Events {
			select {
			case <-ctx.Done():
				result.Close()
				return
			default:
			}
			switch ev.Type {
			case "content":
				textContent += ev.Content
				ch <- StreamEvent{Type: "content", Content: ev.Content}
			case "thinking":
				ch <- StreamEvent{Type: "thinking", Content: ev.Thinking}
			case "tool_call":
				if ev.ToolCall != nil {
					toolCalls = append(toolCalls, *ev.ToolCall)
				}
			case "usage":
				if ev.Usage != nil {
					s.Cost.Add(ev.Usage.PromptTokens, ev.Usage.CompletionTokens)
				}
			case "error":
				ch <- StreamEvent{Type: "error", Content: ev.Error}
				result.Close()
				return
			case "done":
				if ev.StopReason != "" {
					stopReason = ev.StopReason
				}
			}
		}
		result.Close()

		// Handle max_tokens recovery
		if stopReason == "max_tokens" && len(toolCalls) == 0 && recoveryCount < maxRecoveryRetries {
			recoveryCount++
			s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
			s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: "Continue from where you left off."})
			continue
		}

		// No tool calls — done
		if len(toolCalls) == 0 {
			if textContent != "" {
				s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
			}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Append assistant text if any
		if textContent != "" {
			s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
		}

		// Execute tools
		recoveryCount = 0
		for _, tc := range toolCalls {
			ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

			inputJSON, _ := json.Marshal(tc.Arguments)
			output, err := s.registry.Execute(ctx, tc.Name, inputJSON)
			if err != nil {
				output = fmt.Sprintf("Error: %s", err.Error())
			}
			if len(output) > 50000 {
				output = output[:50000] + "\n... (truncated)"
			}

			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}

			// Append as assistant tool_use + user tool_result pair
			s.messages = append(s.messages, client.EyrieMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("[Used tool: %s(%s)]", tc.Name, string(inputJSON)),
			})
			s.messages = append(s.messages, client.EyrieMessage{
				Role:    "user",
				Content: fmt.Sprintf("[Tool result for %s (id: %s)]:\n%s", tc.Name, tc.ID, output),
			})
		}
	}
}

// compact removes older messages, keeping the first and last N.
func (s *Session) compact() {
	if len(s.messages) <= 20 {
		return
	}
	// Keep first 4 messages (initial context) and last 16
	keep := make([]client.EyrieMessage, 0, 21)
	keep = append(keep, s.messages[:4]...)
	keep = append(keep, client.EyrieMessage{
		Role:    "user",
		Content: "[Earlier conversation was compacted to save context. Continue from the recent messages below.]",
	})
	keep = append(keep, s.messages[len(s.messages)-16:]...)
	s.messages = keep
}
