package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hawk/eyrie/client"

	"github.com/GrayCodeAI/hawk/tool"
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
	c := client.NewEyrieClient(&client.EyrieConfig{Provider: detected})
	return &Session{
		client:   c,
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

// LoadMessages restores conversation history (for --resume).
func (s *Session) LoadMessages(msgs []client.EyrieMessage) {
	s.messages = msgs
}

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type     string // "content", "thinking", "tool_use", "tool_result", "tool_ask", "done", "error"
	Content  string
	ToolName string
	ToolID   string
}

// Stream sends the conversation to the LLM and streams events back.
// It handles the agentic loop: if the LLM returns tool_use, it executes
// the tool, appends the result, and calls the LLM again.
func (s *Session) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 64)
	go s.agentLoop(ctx, ch)
	return ch, nil
}

func (s *Session) agentLoop(ctx context.Context, ch chan<- StreamEvent) {
	defer close(ch)

	for {
		opts := client.ChatOptions{
			Provider:  s.provider,
			Model:     s.model,
			MaxTokens: 16384,
		}
		if s.registry != nil {
			opts.Tools = s.registry.EyrieTools()
		}

		msgs := s.buildMessages()
		result, err := s.client.StreamChat(ctx, msgs, opts)
		if err != nil {
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}

		var textContent string
		var toolCalls []client.ToolCall

		for ev := range result.Events {
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
			case "error":
				ch <- StreamEvent{Type: "error", Content: ev.Error}
				result.Close()
				return
			}
		}
		result.Close()

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			if textContent != "" {
				s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
			}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Append assistant message with tool calls (as text for now)
		if textContent != "" {
			s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
		}

		// Execute each tool and append results
		for _, tc := range toolCalls {
			ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

			inputJSON, _ := json.Marshal(tc.Arguments)
			output, err := s.registry.Execute(ctx, tc.Name, inputJSON)
			if err != nil {
				output = fmt.Sprintf("Error: %s", err.Error())
			}

			// Truncate very long outputs
			if len(output) > 50000 {
				output = output[:50000] + "\n... (truncated)"
			}

			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}

			// Append tool result as user message (Anthropic format)
			s.messages = append(s.messages, client.EyrieMessage{
				Role:    "user",
				Content: fmt.Sprintf("[Tool result for %s (id: %s)]:\n%s", tc.Name, tc.ID, output),
			})
		}

		// Loop back — the LLM will see the tool results and continue
	}
}

func (s *Session) buildMessages() []client.EyrieMessage {
	if len(s.messages) == 0 {
		return nil
	}
	var msgs []client.EyrieMessage
	if s.system != "" {
		first := s.messages[0]
		msgs = append(msgs, client.EyrieMessage{Role: first.Role, Content: s.system + "\n\n" + first.Content})
		msgs = append(msgs, s.messages[1:]...)
	} else {
		msgs = append(msgs, s.messages...)
	}
	return msgs
}
