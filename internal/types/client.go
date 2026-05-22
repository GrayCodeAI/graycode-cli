package types

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

type EyrieMessage = client.EyrieMessage
type ChatOptions = client.ChatOptions
type EyrieResponse = client.EyrieResponse
type EyrieUsage = client.EyrieUsage
type StreamResult = client.StreamResult
type EyrieStreamEvent = client.EyrieStreamEvent
type ContinuationConfig = client.ContinuationConfig
type EyrieConfig = client.EyrieConfig
type EyrieTool = client.EyrieTool
type EyrieClient = client.EyrieClient
type Provider = client.Provider

func DefaultContinuationConfig() ContinuationConfig {
	return client.DefaultContinuationConfig()
}

func NewClient(cfg *EyrieConfig) *EyrieClient {
	return client.Client(cfg)
}

func ParseInlineToolCalls(content string) (string, []ToolCall) {
	return client.ParseInlineToolCalls(content)
}

func StreamChatWithContinuation(ctx context.Context, p Provider, messages []EyrieMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error) {
	return client.StreamChatWithContinuation(ctx, p, messages, opts, cfg)
}