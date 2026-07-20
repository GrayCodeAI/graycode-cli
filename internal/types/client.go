package types

import (
	"context"

	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// ContentPart is a provider-neutral multimodal message part. Hawk owns this
// conversation shape; the Eyrie engine adapter translates it at the transport
// boundary. It aliases the canonical contract type.
type ContentPart = llm.ContentPart

// ImageURLPart describes an image URL or data URI.
type ImageURLPart = llm.ImageURLPart

// InputAudioPart describes base64-encoded audio content.
type InputAudioPart = llm.InputAudioPart

// ChatProvider is Hawk's transport-provider interface.
type ChatProvider interface {
	Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error)
	StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error)
	Ping(ctx context.Context) error
	Name() string
}

// ChatClient is the session-level agent-loop client interface.
type ChatClient interface {
	Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error)
	StreamChatContinue(ctx context.Context, messages []EyrieMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error)
}

// ResponseFormat specifies the desired output format for a Hawk runtime request.
type ResponseFormat = llm.ResponseFormat

// ToolChoiceOption controls how the model uses tools.
type ToolChoiceOption = llm.ToolChoiceOption

// EyrieTool is Hawk's runtime tool definition DTO.
type EyrieTool = llm.EyrieTool

// ChatOptions holds Hawk-owned request options for an engine chat call.
type ChatOptions = llm.ChatOptions

// ContinuationConfig controls output continuation behavior for Hawk runtime calls.
type ContinuationConfig = llm.ContinuationConfig

// DefaultContinuationConfig is Hawk's agent-loop continuation policy. Eyrie
// receives these limits through its engine request rather than owning the
// product policy.
func DefaultContinuationConfig() ContinuationConfig {
	return ContinuationConfig{MaxContinuations: 3, MaxTotalTokens: 32000}
}

// ToolCall is Hawk's runtime tool invocation DTO.
type ToolCall = llm.ToolCall

// ToolResult is Hawk's runtime tool result DTO.
type ToolResult = llm.ToolResult

// EyrieUsage tracks token usage for Hawk runtime responses and streams.
type EyrieUsage = llm.EyrieUsage

// ResolvedRoute is Hawk's view of the concrete provider/model route selected
// by the provider engine. Keeping this projection Hawk-owned prevents Eyrie's
// transport DTOs from leaking into product state and observability payloads.
type ResolvedRoute = llm.ResolvedRoute

// EyrieResponse is Hawk's runtime chat response DTO.
type EyrieResponse = llm.EyrieResponse

// EyrieStreamEvent is Hawk's runtime stream event DTO.
type EyrieStreamEvent = llm.EyrieStreamEvent

// StreamResult wraps a Hawk-owned streaming response with cleanup. It aliases
// the canonical contract type; its Close() method and canonical constructor
// (NewStreamResult) live in github.com/GrayCodeAI/hawk-core-contracts/llm.
type StreamResult = llm.StreamResult

// EyrieMessage is Hawk's runtime conversation DTO.
// It intentionally mirrors the engine boundary shape while remaining Hawk-owned.
type EyrieMessage = llm.EyrieMessage
