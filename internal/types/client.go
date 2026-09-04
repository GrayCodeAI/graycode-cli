package types

import (
	"context"

	"github.com/GrayCodeAI/graycode-router/llm"
)

// ContentPart is a provider-neutral multimodal message part. Graycode owns this
// conversation shape; the GraycodeRouter engine adapter translates it at the transport
// boundary. It aliases the canonical contract type.
type ContentPart = llm.ContentPart

// ImageURLPart describes an image URL or data URI.
type ImageURLPart = llm.ImageURLPart

// InputAudioPart describes base64-encoded audio content.
type InputAudioPart = llm.InputAudioPart

// ChatProvider is Graycode's transport-provider interface.
type ChatProvider interface {
	Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error)
	StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error)
	Ping(ctx context.Context) error
	Name() string
}

// ChatClient is the session-level agent-loop client interface.
type ChatClient interface {
	Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error)
	StreamChatContinue(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error)
}

// ResponseFormat specifies the desired output format for a Graycode runtime request.
type ResponseFormat = llm.ResponseFormat

// ToolChoiceOption controls how the model uses tools.
type ToolChoiceOption = llm.ToolChoiceOption

// GraycodeRouterTool is Graycode's runtime tool definition DTO.
type GraycodeRouterTool = llm.GraycodeRouterTool

// ChatOptions holds Graycode-owned request options for an engine chat call.
type ChatOptions = llm.ChatOptions

// ContinuationConfig controls output continuation behavior for Graycode runtime calls.
type ContinuationConfig = llm.ContinuationConfig

// DefaultContinuationConfig is Graycode's agent-loop continuation policy. GraycodeRouter
// receives these limits through its engine request rather than owning the
// product policy.
func DefaultContinuationConfig() ContinuationConfig {
	return ContinuationConfig{MaxContinuations: 3, MaxTotalTokens: 32000}
}

// ToolCall is Graycode's runtime tool invocation DTO.
type ToolCall = llm.ToolCall

// ToolResult is Graycode's runtime tool result DTO.
type ToolResult = llm.ToolResult

// GraycodeRouterUsage tracks token usage for Graycode runtime responses and streams.
type GraycodeRouterUsage = llm.GraycodeRouterUsage

// ResolvedRoute is Graycode's view of the concrete provider/model route selected
// by the provider engine. Keeping this projection Graycode-owned prevents GraycodeRouter's
// transport DTOs from leaking into product state and observability payloads.
type ResolvedRoute = llm.ResolvedRoute

// GraycodeRouterResponse is Graycode's runtime chat response DTO.
type GraycodeRouterResponse = llm.GraycodeRouterResponse

// GraycodeRouterStreamEvent is Graycode's runtime stream event DTO.
type GraycodeRouterStreamEvent = llm.GraycodeRouterStreamEvent

// StreamResult wraps a Graycode-owned streaming response with cleanup. It aliases
// the canonical contract type; its Close() method and canonical constructor
// (NewStreamResult) live in github.com/GrayCodeAI/graycode-router/llm.
type StreamResult = llm.StreamResult

// GraycodeRouterMessage is Graycode's runtime conversation DTO.
// It intentionally mirrors the engine boundary shape while remaining Graycode-owned.
type GraycodeRouterMessage = llm.GraycodeRouterMessage
