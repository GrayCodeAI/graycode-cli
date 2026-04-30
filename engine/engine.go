package engine

import (
	"context"
	"fmt"

	"github.com/hawk/eyrie/client"
)

// Session manages a conversation with an LLM via eyrie.
type Session struct {
	client   *client.EyrieClient
	messages []client.EyrieMessage
	provider string
	model    string
	system   string
}

// NewSession creates a new conversation session.
func NewSession(provider, model, systemPrompt string) *Session {
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
		provider: detected,
		model:    model,
		system:   systemPrompt,
	}
}

// Model returns the active model name.
func (s *Session) Model() string { return s.model }

// Provider returns the active provider name.
func (s *Session) Provider() string { return s.provider }

// AddUser appends a user message to history.
func (s *Session) AddUser(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: content})
}

// AddAssistant appends an assistant message to history.
func (s *Session) AddAssistant(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: content})
}

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type    string // "content", "thinking", "done", "error"
	Content string
}

// Stream sends the current conversation to the LLM and streams events back.
func (s *Session) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	msgs := s.buildMessages()
	opts := client.ChatOptions{
		Provider:  s.provider,
		Model:     s.model,
		MaxTokens: 16384,
	}

	result, err := s.client.StreamChat(ctx, msgs, opts)
	if err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer result.Close()
		for ev := range result.Events {
			switch ev.Type {
			case "content":
				ch <- StreamEvent{Type: "content", Content: ev.Content}
			case "thinking":
				ch <- StreamEvent{Type: "thinking", Content: ev.Thinking}
			case "error":
				ch <- StreamEvent{Type: "error", Content: ev.Error}
			case "done":
				ch <- StreamEvent{Type: "done"}
				return
			}
		}
		ch <- StreamEvent{Type: "done"}
	}()
	return ch, nil
}

func (s *Session) buildMessages() []client.EyrieMessage {
	var msgs []client.EyrieMessage
	if s.system != "" {
		msgs = append(msgs, client.EyrieMessage{Role: "user", Content: s.system + "\n\n" + s.messages[0].Content})
		msgs = append(msgs, s.messages[1:]...)
	} else {
		msgs = s.messages
	}
	return msgs
}
