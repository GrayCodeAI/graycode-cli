package types

import (
	"reflect"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

func TestEyrieMessageClientRoundTrip(t *testing.T) {
	in := []EyrieMessage{
		{
			Role:     "assistant",
			Content:  "done",
			Thinking: "reasoning",
			Images:   []string{"data:image/png;base64,abc"},
			ToolUse: []ToolCall{{
				ID:   "tool_1",
				Name: "Read",
				Arguments: map[string]interface{}{
					"path": "main.go",
				},
			}},
		},
		{
			Role: "user",
			ToolResults: []ToolResult{{
				ToolUseID: "tool_1",
				Content:   "package main",
			}},
		},
	}

	got := FromClientMessages(ToClientMessages(in))
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, in)
	}
}

func TestToolCallClientRoundTrip(t *testing.T) {
	in := []ToolCall{{
		ID:   "tool_1",
		Name: "Read",
		Arguments: map[string]interface{}{
			"path": "main.go",
		},
	}}

	got := FromClientToolCalls(ToClientToolCalls(in))
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("tool call round trip mismatch:\n got: %#v\nwant: %#v", got, in)
	}
}

func TestToolResultClientRoundTrip(t *testing.T) {
	in := []ToolResult{{
		ToolUseID: "tool_1",
		Content:   "ok",
		IsError:   true,
	}}

	got := FromClientToolResults(ToClientToolResults(in))
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("tool result round trip mismatch:\n got: %#v\nwant: %#v", got, in)
	}
}

func TestFromClientMessagePreservesContentParts(t *testing.T) {
	in := client.EyrieMessage{
		Role: "user",
		ContentParts: []client.ContentPart{{
			Type: "text",
			Text: "hello",
		}},
	}

	got := FromClientMessage(in)
	if len(got.ContentParts) != 1 || got.ContentParts[0].Text != "hello" {
		t.Fatalf("ContentParts = %#v, want text part", got.ContentParts)
	}
}

func TestToClientConfig(t *testing.T) {
	cfg := &ClientConfig{
		Provider:   "anthropic",
		APIKey:     "secret",
		BaseURL:    "https://proxy.example",
		Model:      "claude-sonnet",
		MaxRetries: 3,
	}

	got := ToClientConfig(cfg)
	if got == nil {
		t.Fatal("expected non-nil client config")
	}
	if got.Provider != cfg.Provider || got.APIKey != cfg.APIKey || got.BaseURL != cfg.BaseURL || got.Model != cfg.Model || got.MaxRetries != cfg.MaxRetries {
		t.Fatalf("ToClientConfig = %#v, want %#v", got, cfg)
	}
}

func TestWrapClientProvider(t *testing.T) {
	mock := client.NewMockProvider(client.MockModeFixed)
	mock.Response = "wrapped"

	provider := WrapClientProvider(mock)
	if provider == nil {
		t.Fatal("expected non-nil provider adapter")
	}
	if provider.Name() != "mock" {
		t.Fatalf("Name() = %q, want mock", provider.Name())
	}

	resp, err := provider.Chat(t.Context(), []EyrieMessage{{Role: "user", Content: "hi"}}, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "wrapped" {
		t.Fatalf("Content = %q, want wrapped", resp.Content)
	}
}

func TestNewClientPreservesProviderName(t *testing.T) {
	c := NewClient(&ClientConfig{Provider: "openai"})
	if c.Name() != "openai" {
		t.Fatalf("Name() = %q, want openai", c.Name())
	}
}
