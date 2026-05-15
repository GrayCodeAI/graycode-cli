package engine

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

// ChatClient abstracts the LLM client methods used by Session.
// The production implementation is *client.EyrieClient; tests can inject a mock.
type ChatClient interface {
	Chat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error)
	StreamChatContinue(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions, cfg client.ContinuationConfig) (*client.StreamResult, error)
	SetAPIKey(provider, apiKey string)
}
