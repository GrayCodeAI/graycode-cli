package engine

import (
	"context"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// eyrieEngineClient is Hawk's anti-corruption adapter from the product-owned
// ChatClient port to Eyrie's stable engine facade. Hawk continues to own its
// agent loop and conversation state; Eyrie owns model resolution and transport.
type eyrieEngineClient struct {
	engine *eyrieengine.Engine
}

func newEyrieEngineClient(runtime *eyrieengine.Engine) ChatClient {
	return &eyrieEngineClient{engine: runtime}
}

func (c *eyrieEngineClient) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	response, err := c.engine.Generate(ctx, toEngineRequest(messages, opts, types.ContinuationConfig{}))
	if err != nil {
		return nil, err
	}
	return fromEngineResponse(response), nil
}

func (c *eyrieEngineClient) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, continuation types.ContinuationConfig) (*types.StreamResult, error) {
	request := toEngineRequest(messages, opts, continuation)
	request.Requirements.Streaming = true
	stream, err := c.engine.Stream(ctx, request)
	if err != nil {
		return nil, err
	}
	events := make(chan types.EyrieStreamEvent, 64)
	streamCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(events)
		defer stream.Close()
		for stream.Next() {
			event, emit := fromEngineEvent(stream.Event())
			if !emit {
				continue
			}
			select {
			case events <- event:
			case <-streamCtx.Done():
				return
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case events <- types.EyrieStreamEvent{Type: "error", Error: err.Error()}:
			case <-streamCtx.Done():
			}
		}
	}()
	closeFn := func() {
		cancel()
		_ = stream.Close()
	}
	return types.NewStreamResult(events, "", closeFn), nil
}

func (c *eyrieEngineClient) SetAPIKey(_, _ string) {
	// Credentials are owned by Eyrie's injected secret store. The legacy
	// session setter remains a compatibility no-op during migration.
}

// OwnsResilience tells Hawk's compatibility ChatService not to add a second
// retry/rate-limit layer around Eyrie's routed transport.
func (c *eyrieEngineClient) OwnsResilience() bool { return true }

func toEngineRequest(messages []types.EyrieMessage, opts types.ChatOptions, continuation types.ContinuationConfig) eyrieengine.GenerateRequest {
	request := eyrieengine.GenerateRequest{
		Messages:     toEngineMessages(messages),
		SystemPrompt: opts.System,
		Tools:        toEngineTools(opts.Tools),
		Requirements: eyrieengine.Requirements{
			Streaming:      opts.Stream,
			Tools:          len(opts.Tools) > 0,
			Vision:         messagesContainVision(messages),
			StructuredJSON: opts.ResponseFormat != nil || opts.OutputSchema != "",
			Reasoning:      opts.ReasoningEffort != "" || opts.ThinkingBudgetTokens > 0 || opts.ThinkingMode != "" || opts.GLMThinkingEnabled != nil,
		},
		Preference: eyrieengine.Preference{
			PreferredProvider: opts.Provider,
			PreferredModelID:  opts.Model,
		},
		Limits: eyrieengine.Limits{
			MaxOutputTokens:      opts.MaxTokens,
			MaxContinuations:     continuation.MaxContinuations,
			MaxTotalOutputTokens: continuation.MaxTotalTokens,
		},
		Temperature:  opts.Temperature,
		OutputSchema: firstNonEmpty(opts.OutputSchema, responseSchema(opts.ResponseFormat)),
		Options: eyrieengine.GenerationOptions{
			EnableCaching: opts.EnableCaching, ReasoningEffort: opts.ReasoningEffort,
			ThinkingBudgetTokens: opts.ThinkingBudgetTokens, ThinkingMode: opts.ThinkingMode,
			ThinkingDisplay: opts.ThinkingDisplay, GLMThinkingEnabled: opts.GLMThinkingEnabled,
			VirtualKeyID: opts.VirtualKeyID, KimiContextCacheID: opts.KimiContextCacheID,
			KimiCacheResetTTL: opts.KimiCacheResetTTL, TopP: opts.TopP, TopK: opts.TopK,
			StopSequences: append([]string(nil), opts.StopSequences...), ToolChoice: toEngineToolChoice(opts.ToolChoice),
			ServiceTier: opts.ServiceTier, OutputEffort: opts.OutputEffort,
			PresencePenalty: opts.PresencePenalty, FrequencyPenalty: opts.FrequencyPenalty,
			N: opts.N, LogProbs: opts.LogProbs, TopLogProbs: opts.TopLogProbs, Seed: opts.Seed,
			Store: opts.Store, Metadata: cloneMetadata(opts.Metadata), Modalities: append([]string(nil), opts.Modalities...),
			AudioConfig: opts.AudioConfig, Prediction: opts.Prediction, WebSearchOptions: opts.WebSearchOptions,
		},
	}
	return request
}

func toEngineMessages(messages []types.EyrieMessage) []eyrieengine.Message {
	out := make([]eyrieengine.Message, 0, len(messages))
	for _, message := range messages {
		parts := make([]eyrieengine.ContentPart, 0, len(message.ContentParts)+len(message.Images))
		for _, part := range message.ContentParts {
			converted := eyrieengine.ContentPart{Type: part.Type, Text: part.Text}
			if part.ImageURL != nil {
				converted.URL, converted.Detail = part.ImageURL.URL, part.ImageURL.Detail
			}
			if part.InputAudio != nil {
				converted.AudioData, converted.AudioFormat = part.InputAudio.Data, part.InputAudio.Format
			}
			parts = append(parts, converted)
		}
		for _, image := range message.Images {
			parts = append(parts, eyrieengine.ContentPart{Type: "image_url", URL: image})
		}
		calls := make([]eyrieengine.ToolCall, 0, len(message.ToolUse))
		for _, call := range message.ToolUse {
			calls = append(calls, eyrieengine.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		results := make([]eyrieengine.ToolResult, 0, len(message.ToolResults))
		for _, result := range message.ToolResults {
			results = append(results, eyrieengine.ToolResult{ToolUseID: result.ToolUseID, Content: result.Content, IsError: result.IsError})
		}
		out = append(out, eyrieengine.Message{
			Role: message.Role, Content: message.Content, Thinking: message.Thinking,
			ContentParts: parts, ToolCalls: calls, ToolResults: results,
		})
	}
	return out
}

func toEngineTools(tools []types.EyrieTool) []eyrieengine.Tool {
	out := make([]eyrieengine.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, eyrieengine.Tool{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters})
	}
	return out
}

func toEngineToolChoice(choice *types.ToolChoiceOption) *eyrieengine.ToolChoice {
	if choice == nil {
		return nil
	}
	return &eyrieengine.ToolChoice{Type: choice.Type, Name: choice.Name, DisableParallelToolUse: choice.DisableParallelToolUse}
}

func fromEngineResponse(response *eyrieengine.GenerateResponse) *types.EyrieResponse {
	if response == nil {
		return &types.EyrieResponse{}
	}
	calls := make([]types.ToolCall, 0, len(response.ToolCalls))
	for _, call := range response.ToolCalls {
		calls = append(calls, types.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
	}
	return &types.EyrieResponse{
		Content: response.Content, Thinking: response.Thinking, ToolCalls: calls,
		FinishReason: response.FinishReason, RequestID: response.RequestID, Usage: fromEngineUsage(response.Usage),
	}
}

func fromEngineEvent(event eyrieengine.Event) (types.EyrieStreamEvent, bool) {
	out := types.EyrieStreamEvent{
		Content: event.Content, Thinking: event.Thinking, RequestID: event.RequestID,
		Usage: fromEngineUsage(event.Usage), StopReason: event.StopReason, TTFTms: event.TTFTMillis,
	}
	switch event.Type {
	case eyrieengine.EventRouteSelected:
		return types.EyrieStreamEvent{}, false
	case eyrieengine.EventContentDelta:
		out.Type = "content"
	case eyrieengine.EventThinkingDelta:
		out.Type = "thinking"
	case eyrieengine.EventToolCallStart, eyrieengine.EventToolCallDone:
		out.Type = "tool_call"
	case eyrieengine.EventToolCallDelta:
		out.Type = "tool_input_delta"
	case eyrieengine.EventUsage:
		out.Type = "usage"
	case eyrieengine.EventTTFT:
		out.Type = "ttft"
		out.TTFT = event.TTFTMillis
	case eyrieengine.EventDone:
		out.Type = "done"
	case eyrieengine.EventContinuation:
		out.Type = "continuation"
	case eyrieengine.EventWarning:
		out.Type, out.Content = "warning", event.Warning
	default:
		out.Type = string(event.Type)
	}
	if event.ToolCall != nil {
		out.ToolCall = &types.ToolCall{ID: event.ToolCall.ID, Name: event.ToolCall.Name, Arguments: event.ToolCall.Arguments}
	}
	return out, true
}

func fromEngineUsage(usage *eyrieengine.Usage) *types.EyrieUsage {
	if usage == nil {
		return nil
	}
	return &types.EyrieUsage{
		PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens, TotalTokens: usage.TotalTokens,
		CacheCreationTokens: usage.CacheCreationTokens, CacheReadTokens: usage.CacheReadTokens, ThinkingTokens: usage.ThinkingTokens,
	}
}

func responseSchema(format *types.ResponseFormat) string {
	if format == nil {
		return ""
	}
	return format.Schema
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneMetadata(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func messagesContainVision(messages []types.EyrieMessage) bool {
	for _, message := range messages {
		if len(message.Images) > 0 {
			return true
		}
		for _, part := range message.ContentParts {
			if part.ImageURL != nil || part.Type == "image_url" {
				return true
			}
		}
	}
	return false
}
