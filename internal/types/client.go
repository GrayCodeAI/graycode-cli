package types

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

type (
	EyrieMessage       = client.EyrieMessage
	ChatOptions        = client.ChatOptions
	EyrieResponse      = client.EyrieResponse
	EyrieUsage         = client.EyrieUsage
	StreamResult       = client.StreamResult
	EyrieStreamEvent   = client.EyrieStreamEvent
	ContinuationConfig = client.ContinuationConfig
	EyrieConfig        = client.EyrieConfig
	EyrieTool          = client.EyrieTool
	EyrieClient        = client.EyrieClient
	Provider           = client.Provider
	ResponseFormat     = client.ResponseFormat
)

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
