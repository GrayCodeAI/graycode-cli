package types

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

type (
	ContentPart    = client.ContentPart
	ImageURLPart   = client.ImageURLPart
	InputAudioPart = client.InputAudioPart
)

// ClientConfig is Hawk-owned client construction config at the transport edge.
type ClientConfig struct {
	Provider   string `json:"provider,omitempty"`
	APIKey     string `json:"-"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
}

// ChatProvider is Hawk's transport-provider interface.
type ChatProvider interface {
	Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error)
	StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error)
	Ping(ctx context.Context) error
	Name() string
}

// ResponseFormat specifies the desired output format for a Hawk runtime request.
type ResponseFormat struct {
	Type   string `json:"type"`
	Schema string `json:"schema,omitempty"`
}

// ToolChoiceOption controls how the model uses tools.
type ToolChoiceOption struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

// EyrieTool is Hawk's runtime tool definition DTO.
type EyrieTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatOptions holds Hawk-owned request options for a runtime chat call.
type ChatOptions struct {
	Provider             string            `json:"provider,omitempty"`
	Model                string            `json:"model,omitempty"`
	Temperature          *float64          `json:"temperature,omitempty"`
	MaxTokens            int               `json:"max_tokens,omitempty"`
	Stream               bool              `json:"stream,omitempty"`
	Tools                []EyrieTool       `json:"tools,omitempty"`
	System               string            `json:"system,omitempty"`
	EnableCaching        bool              `json:"enable_caching,omitempty"`
	ResponseFormat       *ResponseFormat   `json:"response_format,omitempty"`
	ReasoningEffort      string            `json:"reasoning_effort,omitempty"`
	ThinkingBudgetTokens int               `json:"thinking_budget_tokens,omitempty"`
	ThinkingMode         string            `json:"thinking_mode,omitempty"`
	ThinkingDisplay      string            `json:"thinking_display,omitempty"`
	GLMThinkingEnabled   *bool             `json:"glm_thinking_enabled,omitempty"`
	VirtualKeyID         string            `json:"virtual_key_id,omitempty"`
	KimiContextCacheID   string            `json:"kimi_context_cache_id,omitempty"`
	KimiCacheResetTTL    bool              `json:"kimi_cache_reset_ttl,omitempty"`
	TopP                 *float64          `json:"top_p,omitempty"`
	TopK                 *int              `json:"top_k,omitempty"`
	StopSequences        []string          `json:"stop_sequences,omitempty"`
	ToolChoice           *ToolChoiceOption `json:"tool_choice,omitempty"`
	MetadataUserID       string            `json:"metadata_user_id,omitempty"`
	ServiceTier          string            `json:"service_tier,omitempty"`
	OutputEffort         string            `json:"output_effort,omitempty"`
	OutputSchema         string            `json:"output_schema,omitempty"`
	PresencePenalty      *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty     *float64          `json:"frequency_penalty,omitempty"`
	N                    *int              `json:"n,omitempty"`
	LogProbs             *bool             `json:"logprobs,omitempty"`
	TopLogProbs          *int              `json:"top_logprobs,omitempty"`
	Seed                 *int              `json:"seed,omitempty"`
	Store                *bool             `json:"store,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	Modalities           []string          `json:"modalities,omitempty"`
	AudioConfig          string            `json:"audio_config,omitempty"`
	Prediction           string            `json:"prediction,omitempty"`
	WebSearchOptions     string            `json:"web_search_options,omitempty"`
}

// ContinuationConfig controls output continuation behavior for Hawk runtime calls.
type ContinuationConfig struct {
	MaxContinuations int
	MaxTotalTokens   int
}

// ToolCall is Hawk's runtime tool invocation DTO.
type ToolCall struct {
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResult is Hawk's runtime tool result DTO.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// EyrieUsage tracks token usage for Hawk runtime responses and streams.
type EyrieUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
	ThinkingTokens      int `json:"thinking_tokens,omitempty"`
}

// EyrieResponse is Hawk's runtime chat response DTO.
type EyrieResponse struct {
	Content        string      `json:"content"`
	Thinking       string      `json:"thinking,omitempty"`
	Usage          *EyrieUsage `json:"usage,omitempty"`
	ToolCalls      []ToolCall  `json:"tool_calls,omitempty"`
	FinishReason   string      `json:"finish_reason"`
	RequestID      string      `json:"request_id,omitempty"`
	OrganizationID string      `json:"organization_id,omitempty"`
}

// EyrieStreamEvent is Hawk's runtime stream event DTO.
type EyrieStreamEvent struct {
	Type       string      `json:"type"`
	Content    string      `json:"content,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	Thinking   string      `json:"thinking,omitempty"`
	Error      string      `json:"error,omitempty"`
	RequestID  string      `json:"request_id,omitempty"`
	Usage      *EyrieUsage `json:"usage,omitempty"`
	StopReason string      `json:"stop_reason,omitempty"`
	TTFTms     int         `json:"ttft_ms,omitempty"`
	TTFT       int         `json:"ttft,omitempty"`
}

// StreamResult wraps a Hawk-owned streaming response with cleanup.
type StreamResult struct {
	Events    <-chan EyrieStreamEvent
	RequestID string
	cancel    context.CancelFunc
}

// Close stops the stream and releases resources.
func (sr *StreamResult) Close() {
	if sr != nil && sr.cancel != nil {
		sr.cancel()
	}
}

// EyrieMessage is Hawk's runtime conversation DTO.
// It intentionally mirrors the provider runtime shape while remaining Hawk-owned.
type EyrieMessage struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	Thinking     string        `json:"thinking,omitempty"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	Images       []string      `json:"images,omitempty"`
	ToolUse      []ToolCall    `json:"tool_use,omitempty"`
	ToolResults  []ToolResult  `json:"tool_results,omitempty"`
}

// EyrieClient adapts eyrie's client to Hawk-owned message DTOs.
type EyrieClient struct {
	inner        *client.EyrieClient
	providerName string
}

type providerAdapter struct {
	inner client.Provider
}

func DefaultContinuationConfig() ContinuationConfig {
	cfg := client.DefaultContinuationConfig()
	return ContinuationConfig{
		MaxContinuations: cfg.MaxContinuations,
		MaxTotalTokens:   cfg.MaxTotalTokens,
	}
}

func DetectProvider() string {
	return client.DetectProvider()
}

func RegisterDynamicProvider(name, baseURL, apiKeyEnv string) error {
	return client.RegisterDynamicProvider(name, baseURL, apiKeyEnv)
}

func NewClient(cfg *ClientConfig) *EyrieClient {
	providerName := ""
	if cfg != nil {
		providerName = cfg.Provider
	}
	return &EyrieClient{
		inner:        client.Client(ToClientConfig(cfg)),
		providerName: providerName,
	}
}

func ParseInlineToolCalls(content string) (string, []ToolCall) {
	text, calls := client.ParseInlineToolCalls(content)
	return text, FromClientToolCalls(calls)
}

func WrapClientProvider(p client.Provider) ChatProvider {
	if p == nil {
		return nil
	}
	return &providerAdapter{inner: p}
}

func StreamChatWithContinuation(ctx context.Context, p ChatProvider, messages []EyrieMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error) {
	if p == nil {
		return nil, nil
	}
	if adapted, ok := p.(*providerAdapter); ok {
		stream, err := client.StreamChatWithContinuation(ctx, adapted.inner, ToClientMessages(messages), ToClientChatOptions(opts), ToClientContinuationConfig(cfg))
		if err != nil {
			return nil, err
		}
		return FromClientStreamResult(stream), nil
	}
	stream, err := p.StreamChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *EyrieClient) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	resp, err := c.inner.Chat(ctx, ToClientMessages(messages), ToClientChatOptions(opts))
	if err != nil {
		return nil, err
	}
	return FromClientResponse(resp), nil
}

func (c *EyrieClient) StreamChatContinue(ctx context.Context, messages []EyrieMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error) {
	stream, err := c.inner.StreamChatContinue(ctx, ToClientMessages(messages), ToClientChatOptions(opts), ToClientContinuationConfig(cfg))
	if err != nil {
		return nil, err
	}
	return FromClientStreamResult(stream), nil
}

func (c *EyrieClient) SetAPIKey(provider, apiKey string) {
	c.inner.SetAPIKey(provider, apiKey)
}

func (c *EyrieClient) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	stream, err := c.inner.StreamChat(ctx, ToClientMessages(messages), ToClientChatOptions(opts))
	if err != nil {
		return nil, err
	}
	return FromClientStreamResult(stream), nil
}

func (c *EyrieClient) Ping(ctx context.Context) error {
	return c.inner.Ping(ctx, "")
}

func (c *EyrieClient) Name() string {
	return c.providerName
}

func (c *EyrieClient) GetProviders() []string {
	return c.inner.GetProviders()
}

func (p *providerAdapter) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	resp, err := p.inner.Chat(ctx, ToClientMessages(messages), ToClientChatOptions(opts))
	if err != nil {
		return nil, err
	}
	return FromClientResponse(resp), nil
}

func (p *providerAdapter) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	stream, err := p.inner.StreamChat(ctx, ToClientMessages(messages), ToClientChatOptions(opts))
	if err != nil {
		return nil, err
	}
	return FromClientStreamResult(stream), nil
}

func (p *providerAdapter) Ping(ctx context.Context) error {
	return p.inner.Ping(ctx)
}

func (p *providerAdapter) Name() string {
	return p.inner.Name()
}

// ToClientConfig converts Hawk-owned transport config into the provider-runtime shape.
func ToClientConfig(cfg *ClientConfig) *client.EyrieConfig {
	if cfg == nil {
		return nil
	}
	return &client.EyrieConfig{
		Provider:   cfg.Provider,
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		MaxRetries: cfg.MaxRetries,
	}
}

// ToClientResponseFormat converts a Hawk runtime response format into the provider-runtime shape.
func ToClientResponseFormat(format *ResponseFormat) *client.ResponseFormat {
	if format == nil {
		return nil
	}
	return &client.ResponseFormat{
		Type:   format.Type,
		Schema: format.Schema,
	}
}

// ToClientToolChoiceOption converts a Hawk runtime tool choice into the provider-runtime shape.
func ToClientToolChoiceOption(choice *ToolChoiceOption) *client.ToolChoiceOption {
	if choice == nil {
		return nil
	}
	return &client.ToolChoiceOption{
		Type:                   choice.Type,
		Name:                   choice.Name,
		DisableParallelToolUse: choice.DisableParallelToolUse,
	}
}

// ToClientEyrieTool converts a Hawk runtime tool definition into the provider-runtime shape.
func ToClientEyrieTool(tool EyrieTool) client.EyrieTool {
	return client.EyrieTool{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  tool.Parameters,
	}
}

// ToClientEyrieTools converts Hawk runtime tool definitions into provider-runtime tool definitions.
func ToClientEyrieTools(tools []EyrieTool) []client.EyrieTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]client.EyrieTool, len(tools))
	for i, tool := range tools {
		out[i] = ToClientEyrieTool(tool)
	}
	return out
}

// ToClientChatOptions converts Hawk runtime chat options into the provider-runtime shape.
func ToClientChatOptions(opts ChatOptions) client.ChatOptions {
	return client.ChatOptions{
		Provider:             opts.Provider,
		Model:                opts.Model,
		Temperature:          opts.Temperature,
		MaxTokens:            opts.MaxTokens,
		Stream:               opts.Stream,
		Tools:                ToClientEyrieTools(opts.Tools),
		System:               opts.System,
		EnableCaching:        opts.EnableCaching,
		ResponseFormat:       ToClientResponseFormat(opts.ResponseFormat),
		ReasoningEffort:      opts.ReasoningEffort,
		ThinkingBudgetTokens: opts.ThinkingBudgetTokens,
		ThinkingMode:         opts.ThinkingMode,
		ThinkingDisplay:      opts.ThinkingDisplay,
		GLMThinkingEnabled:   opts.GLMThinkingEnabled,
		VirtualKeyID:         opts.VirtualKeyID,
		KimiContextCacheID:   opts.KimiContextCacheID,
		KimiCacheResetTTL:    opts.KimiCacheResetTTL,
		TopP:                 opts.TopP,
		TopK:                 opts.TopK,
		StopSequences:        opts.StopSequences,
		ToolChoice:           ToClientToolChoiceOption(opts.ToolChoice),
		MetadataUserID:       opts.MetadataUserID,
		ServiceTier:          opts.ServiceTier,
		OutputEffort:         opts.OutputEffort,
		OutputSchema:         opts.OutputSchema,
		PresencePenalty:      opts.PresencePenalty,
		FrequencyPenalty:     opts.FrequencyPenalty,
		N:                    opts.N,
		LogProbs:             opts.LogProbs,
		TopLogProbs:          opts.TopLogProbs,
		Seed:                 opts.Seed,
		Store:                opts.Store,
		Metadata:             opts.Metadata,
		Modalities:           opts.Modalities,
		AudioConfig:          opts.AudioConfig,
		Prediction:           opts.Prediction,
		WebSearchOptions:     opts.WebSearchOptions,
	}
}

// ToClientContinuationConfig converts Hawk runtime continuation settings into the provider-runtime shape.
func ToClientContinuationConfig(cfg ContinuationConfig) client.ContinuationConfig {
	return client.ContinuationConfig{
		MaxContinuations: cfg.MaxContinuations,
		MaxTotalTokens:   cfg.MaxTotalTokens,
	}
}

// ToClientToolCall converts a Hawk runtime tool call into the provider-runtime shape.
func ToClientToolCall(tc ToolCall) client.ToolCall {
	return client.ToolCall{
		ID:        tc.ID,
		Name:      tc.Name,
		Arguments: tc.Arguments,
	}
}

// ToClientToolCalls converts Hawk runtime tool calls into provider-runtime tool calls.
func ToClientToolCalls(calls []ToolCall) []client.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]client.ToolCall, len(calls))
	for i, tc := range calls {
		out[i] = ToClientToolCall(tc)
	}
	return out
}

// FromClientToolCall converts a provider-runtime tool call into Hawk's runtime shape.
func FromClientToolCall(tc client.ToolCall) ToolCall {
	return ToolCall{
		ID:        tc.ID,
		Name:      tc.Name,
		Arguments: tc.Arguments,
	}
}

// FromClientToolCalls converts provider-runtime tool calls into Hawk runtime tool calls.
func FromClientToolCalls(calls []client.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, len(calls))
	for i, tc := range calls {
		out[i] = FromClientToolCall(tc)
	}
	return out
}

// ToClientToolResult converts a Hawk runtime tool result into the provider-runtime shape.
func ToClientToolResult(tr ToolResult) client.ToolResult {
	return client.ToolResult{
		ToolUseID: tr.ToolUseID,
		Content:   tr.Content,
		IsError:   tr.IsError,
	}
}

// ToClientToolResults converts Hawk runtime tool results into provider-runtime tool results.
func ToClientToolResults(results []ToolResult) []client.ToolResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]client.ToolResult, len(results))
	for i, tr := range results {
		out[i] = ToClientToolResult(tr)
	}
	return out
}

// FromClientToolResult converts a provider-runtime tool result into Hawk's runtime shape.
func FromClientToolResult(tr client.ToolResult) ToolResult {
	return ToolResult{
		ToolUseID: tr.ToolUseID,
		Content:   tr.Content,
		IsError:   tr.IsError,
	}
}

// FromClientToolResults converts provider-runtime tool results into Hawk runtime tool results.
func FromClientToolResults(results []client.ToolResult) []ToolResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]ToolResult, len(results))
	for i, tr := range results {
		out[i] = FromClientToolResult(tr)
	}
	return out
}

// ToClientUsage converts a Hawk runtime usage payload into the provider-runtime shape.
func ToClientUsage(usage *EyrieUsage) *client.EyrieUsage {
	if usage == nil {
		return nil
	}
	return &client.EyrieUsage{
		PromptTokens:        usage.PromptTokens,
		CompletionTokens:    usage.CompletionTokens,
		TotalTokens:         usage.TotalTokens,
		CacheCreationTokens: usage.CacheCreationTokens,
		CacheReadTokens:     usage.CacheReadTokens,
		ThinkingTokens:      usage.ThinkingTokens,
	}
}

// FromClientUsage converts a provider-runtime usage payload into Hawk's runtime shape.
func FromClientUsage(usage *client.EyrieUsage) *EyrieUsage {
	if usage == nil {
		return nil
	}
	return &EyrieUsage{
		PromptTokens:        usage.PromptTokens,
		CompletionTokens:    usage.CompletionTokens,
		TotalTokens:         usage.TotalTokens,
		CacheCreationTokens: usage.CacheCreationTokens,
		CacheReadTokens:     usage.CacheReadTokens,
		ThinkingTokens:      usage.ThinkingTokens,
	}
}

// FromClientResponse converts a provider-runtime response into Hawk's runtime shape.
func FromClientResponse(resp *client.EyrieResponse) *EyrieResponse {
	if resp == nil {
		return nil
	}
	return &EyrieResponse{
		Content:        resp.Content,
		Thinking:       resp.Thinking,
		Usage:          FromClientUsage(resp.Usage),
		ToolCalls:      FromClientToolCalls(resp.ToolCalls),
		FinishReason:   resp.FinishReason,
		RequestID:      resp.RequestID,
		OrganizationID: resp.OrganizationID,
	}
}

// ToClientStreamEvent converts a Hawk runtime stream event into the provider-runtime shape.
func ToClientStreamEvent(ev EyrieStreamEvent) client.EyrieStreamEvent {
	var toolCall *client.ToolCall
	if ev.ToolCall != nil {
		tc := ToClientToolCall(*ev.ToolCall)
		toolCall = &tc
	}
	return client.EyrieStreamEvent{
		Type:       ev.Type,
		Content:    ev.Content,
		ToolCall:   toolCall,
		Thinking:   ev.Thinking,
		Error:      ev.Error,
		RequestID:  ev.RequestID,
		Usage:      ToClientUsage(ev.Usage),
		StopReason: ev.StopReason,
		TTFTms:     ev.TTFTms,
		TTFT:       ev.TTFT,
	}
}

// FromClientStreamEvent converts a provider-runtime stream event into Hawk's runtime shape.
func FromClientStreamEvent(ev client.EyrieStreamEvent) EyrieStreamEvent {
	var toolCall *ToolCall
	if ev.ToolCall != nil {
		tc := FromClientToolCall(*ev.ToolCall)
		toolCall = &tc
	}
	return EyrieStreamEvent{
		Type:       ev.Type,
		Content:    ev.Content,
		ToolCall:   toolCall,
		Thinking:   ev.Thinking,
		Error:      ev.Error,
		RequestID:  ev.RequestID,
		Usage:      FromClientUsage(ev.Usage),
		StopReason: ev.StopReason,
		TTFTms:     ev.TTFTms,
		TTFT:       ev.TTFT,
	}
}

// FromClientStreamResult converts a provider-runtime stream result into Hawk's runtime shape.
func FromClientStreamResult(stream *client.StreamResult) *StreamResult {
	if stream == nil {
		return nil
	}
	out := make(chan EyrieStreamEvent, 64)
	go func() {
		defer close(out)
		for ev := range stream.Events {
			out <- FromClientStreamEvent(ev)
		}
	}()
	return &StreamResult{
		Events:    out,
		RequestID: stream.RequestID,
		cancel:    stream.Close,
	}
}

// ToClientMessage converts a Hawk runtime message into the provider-runtime shape.
func ToClientMessage(msg EyrieMessage) client.EyrieMessage {
	return client.EyrieMessage{
		Role:         msg.Role,
		Content:      msg.Content,
		Thinking:     msg.Thinking,
		ContentParts: msg.ContentParts,
		Images:       msg.Images,
		ToolUse:      ToClientToolCalls(msg.ToolUse),
		ToolResults:  ToClientToolResults(msg.ToolResults),
	}
}

// ToClientMessages converts Hawk runtime messages into provider-runtime messages.
func ToClientMessages(messages []EyrieMessage) []client.EyrieMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]client.EyrieMessage, len(messages))
	for i, msg := range messages {
		out[i] = ToClientMessage(msg)
	}
	return out
}

// FromClientMessage converts a provider-runtime message into Hawk's runtime shape.
func FromClientMessage(msg client.EyrieMessage) EyrieMessage {
	return EyrieMessage{
		Role:         msg.Role,
		Content:      msg.Content,
		Thinking:     msg.Thinking,
		ContentParts: msg.ContentParts,
		Images:       msg.Images,
		ToolUse:      FromClientToolCalls(msg.ToolUse),
		ToolResults:  FromClientToolResults(msg.ToolResults),
	}
}

// FromClientMessages converts provider-runtime messages into Hawk runtime messages.
func FromClientMessages(messages []client.EyrieMessage) []EyrieMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]EyrieMessage, len(messages))
	for i, msg := range messages {
		out[i] = FromClientMessage(msg)
	}
	return out
}
