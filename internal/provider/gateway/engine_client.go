// Package gateway is Hawk's single boundary to Eyrie's provider runtime. It is
// the only package that imports Eyrie; everything else speaks the hawk-owned
// Provider interface and the internal/types DTOs.
//
// hawk = product face (UX/agent/sessions) · eyrie = provider engine
// One-way dependency only: eyrie never imports hawk. See README ecosystems.
package gateway

import (
	"context"
	"fmt"
	"log/slog"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// Provider is Hawk's hawk-owned view of the Eyrie engine: a composition of the
// role interfaces below. It wraps the concrete *eyrieengine.Engine (whose fields
// are unexported and therefore not directly mockable) so Hawk tests can inject a
// stub. Splitting into roles lets callers and stubs depend only on the facet they
// use (e.g. the ChatClient path needs only Generator); Provider stays the full
// surface so nothing that depends on it breaks.
type Provider interface {
	Generator
	NativeCompactor
	ModelCatalog
	CredentialManager
	SelectionManager
	GatewayInspector
	CatalogMaintenance
}

// Generator is the chat transport facet: the only part the ChatClient path uses.
type Generator interface {
	Generate(ctx context.Context, req eyrieengine.GenerateRequest) (*eyrieengine.GenerateResponse, error)
	Stream(ctx context.Context, req eyrieengine.GenerateRequest) (eyrieengine.EventStreamer, error)
}

// NativeCompactor is the provider-native-compaction facet.
type NativeCompactor interface {
	SupportsNativeCompaction(ctx context.Context, provider, model string) bool
	CompactNative(ctx context.Context, req eyrieengine.NativeCompactionRequest) (string, error)
}

// ModelCatalog is the model-discovery facet (used by routing + config).
type ModelCatalog interface {
	ListModels(ctx context.Context, providerID string, refresh bool) ([]eyrieengine.Model, error)
	ListLiveModels(ctx context.Context, providerID string) ([]eyrieengine.Model, error)
	ListPublicModels(ctx context.Context, providerID string) ([]eyrieengine.Model, error)
	ModelInfo(ctx context.Context, modelID string) (eyrieengine.Model, bool, error)
	ModelProviders(ctx context.Context) ([]string, error)
	DefaultModel(ctx context.Context, provider, fallback string) string
	PreferredModel(ctx context.Context, provider string, class eyrieengine.ModelClass, fallback string) string
	PreferredModels(ctx context.Context, primaryProvider string, class eyrieengine.ModelClass, limit int) []string
	ModelClassOf(ctx context.Context, modelID string) eyrieengine.ModelClass
	ProviderForModel(ctx context.Context, modelID string) string
	PrimaryModel(ctx context.Context) string
	ModelNames(ctx context.Context) []string
	Catalog(ctx context.Context) (eyrieengine.CatalogSnapshot, error)
}

// CredentialManager is the key/credential facet (config only).
type CredentialManager interface {
	SaveCredential(ctx context.Context, providerID, secret string) (eyrieengine.CredentialStatus, error)
	RemoveCredential(ctx context.Context, providerID string) error
	CredentialStatus(ctx context.Context, providerID string) (eyrieengine.CredentialStatus, error)
	SaveCredentialEnv(ctx context.Context, envVar, secret string) error
	HasCredentialEnv(ctx context.Context, envVar string) bool
	CredentialEnvKeys(providerID string) []string
	ResolveCredential(ctx context.Context, secret string) eyrieengine.CredentialResolution
	CredentialProviders(context.Context) []eyrieengine.CredentialProvider
	ApplyCredentials(ctx context.Context, providerID string) (eyrieengine.CatalogSnapshot, error)
}

// SelectionManager is the get/set selection facet (config only).
type SelectionManager interface {
	ActiveSelection(ctx context.Context) eyrieengine.Route
	EffectiveSelection(ctx context.Context, opts eyrieengine.SelectionOptions) eyrieengine.Selection
	SetActiveProvider(ctx context.Context, provider string) error
	SetActiveModel(ctx context.Context, modelID string) error
	SetSelection(ctx context.Context, provider, modelID string) error
	ClearSelection(ctx context.Context) error
}

// GatewayInspector is the gateway/deployment/catalog-state facet (config only).
type GatewayInspector interface {
	GatewayDefinitions() []eyrieengine.Gateway
	Gateways(ctx context.Context) []eyrieengine.Gateway
	GatewayRegion(providerID string) (label string, required bool)
	SetGatewayRegion(ctx context.Context, providerID, value string) error
	GatewayForModel(ctx context.Context, modelID string) string
	CanonicalModel(ctx context.Context, modelID string) string
	DeploymentRoutingEnabled(override *bool) bool
	DeploymentStatus(ctx context.Context, activeModel string) (string, error)
	DeploymentSummary(ctx context.Context, activeModel string) (eyrieengine.DeploymentSummary, error)
	RoutingPreview(ctx context.Context, modelID string) (string, error)
}

// CatalogMaintenance is the refresh/preflight/security facet (config only).
type CatalogMaintenance interface {
	RefreshCatalog(ctx context.Context, providerID string) (eyrieengine.CatalogSnapshot, error)
	CatalogHealth(ctx context.Context) eyrieengine.CatalogHealth
	StatePaths() eyrieengine.StatePaths
	DefaultProviderFilter(ctx context.Context) string
	PreflightWithOptions(ctx context.Context, opts eyrieengine.PreflightOptions) eyrieengine.PreflightReport
	ProviderStateSecurityStatus() eyrieengine.ProviderStateSecurity
	MigrateProviderSecrets() error
}

// engineProvider is the production Provider: a thin wrapper over Eyrie's
// concrete engine facade.
type engineProvider struct {
	eng *eyrieengine.Engine
}

func newEngineProvider(eng *eyrieengine.Engine) *engineProvider {
	return &engineProvider{eng: eng}
}

func (p *engineProvider) Generate(ctx context.Context, req eyrieengine.GenerateRequest) (*eyrieengine.GenerateResponse, error) {
	return p.eng.Generate(ctx, req)
}

func (p *engineProvider) Stream(ctx context.Context, req eyrieengine.GenerateRequest) (eyrieengine.EventStreamer, error) {
	return p.eng.Stream(ctx, req)
}

func (p *engineProvider) ListModels(ctx context.Context, providerID string, refresh bool) ([]eyrieengine.Model, error) {
	return p.eng.ListModels(ctx, providerID, refresh)
}

func (p *engineProvider) ListLiveModels(ctx context.Context, providerID string) ([]eyrieengine.Model, error) {
	return p.eng.ListLiveModels(ctx, providerID)
}

func (p *engineProvider) ListPublicModels(ctx context.Context, providerID string) ([]eyrieengine.Model, error) {
	return p.eng.ListPublicModels(ctx, providerID)
}

func (p *engineProvider) ModelInfo(ctx context.Context, modelID string) (eyrieengine.Model, bool, error) {
	return p.eng.ModelInfo(ctx, modelID)
}

func (p *engineProvider) ModelProviders(ctx context.Context) ([]string, error) {
	return p.eng.ModelProviders(ctx)
}

func (p *engineProvider) DefaultModel(ctx context.Context, provider, fallback string) string {
	return p.eng.DefaultModel(ctx, provider, fallback)
}

func (p *engineProvider) PreferredModel(ctx context.Context, provider string, class eyrieengine.ModelClass, fallback string) string {
	return p.eng.PreferredModel(ctx, provider, class, fallback)
}

func (p *engineProvider) PreferredModels(ctx context.Context, primaryProvider string, class eyrieengine.ModelClass, limit int) []string {
	return p.eng.PreferredModels(ctx, primaryProvider, class, limit)
}

func (p *engineProvider) ModelClassOf(ctx context.Context, modelID string) eyrieengine.ModelClass {
	return p.eng.ModelClassOf(ctx, modelID)
}

func (p *engineProvider) ProviderForModel(ctx context.Context, modelID string) string {
	return p.eng.ProviderForModel(ctx, modelID)
}

func (p *engineProvider) PrimaryModel(ctx context.Context) string {
	return p.eng.PrimaryModel(ctx)
}

func (p *engineProvider) ModelNames(ctx context.Context) []string {
	return p.eng.ModelNames(ctx)
}

func (p *engineProvider) StatePaths() eyrieengine.StatePaths {
	return p.eng.StatePaths()
}

func (p *engineProvider) DefaultProviderFilter(ctx context.Context) string {
	return p.eng.DefaultProviderFilter(ctx)
}

func (p *engineProvider) Catalog(ctx context.Context) (eyrieengine.CatalogSnapshot, error) {
	return p.eng.Catalog(ctx)
}

func (p *engineProvider) RefreshCatalog(ctx context.Context, providerID string) (eyrieengine.CatalogSnapshot, error) {
	return p.eng.RefreshCatalog(ctx, providerID)
}

func (p *engineProvider) ApplyCredentials(ctx context.Context, providerID string) (eyrieengine.CatalogSnapshot, error) {
	return p.eng.ApplyCredentials(ctx, providerID)
}

func (p *engineProvider) SaveCredential(ctx context.Context, providerID, secret string) (eyrieengine.CredentialStatus, error) {
	return p.eng.SaveCredential(ctx, providerID, secret)
}

func (p *engineProvider) RemoveCredential(ctx context.Context, providerID string) error {
	return p.eng.RemoveCredential(ctx, providerID)
}

func (p *engineProvider) CredentialStatus(ctx context.Context, providerID string) (eyrieengine.CredentialStatus, error) {
	return p.eng.CredentialStatus(ctx, providerID)
}

func (p *engineProvider) SaveCredentialEnv(ctx context.Context, envVar, secret string) error {
	return p.eng.SaveCredentialEnv(ctx, envVar, secret)
}

func (p *engineProvider) HasCredentialEnv(ctx context.Context, envVar string) bool {
	return p.eng.HasCredentialEnv(ctx, envVar)
}

func (p *engineProvider) CredentialEnvKeys(providerID string) []string {
	return p.eng.CredentialEnvKeys(providerID)
}

func (p *engineProvider) ResolveCredential(ctx context.Context, secret string) eyrieengine.CredentialResolution {
	return p.eng.ResolveCredential(ctx, secret)
}

func (p *engineProvider) CredentialProviders(ctx context.Context) []eyrieengine.CredentialProvider {
	return p.eng.CredentialProviders(ctx)
}

func (p *engineProvider) GatewayDefinitions() []eyrieengine.Gateway {
	return p.eng.GatewayDefinitions()
}

func (p *engineProvider) Gateways(ctx context.Context) []eyrieengine.Gateway {
	return p.eng.Gateways(ctx)
}

func (p *engineProvider) GatewayRegion(providerID string) (string, bool) {
	return p.eng.GatewayRegion(providerID)
}

func (p *engineProvider) SetGatewayRegion(ctx context.Context, providerID, value string) error {
	return p.eng.SetGatewayRegion(ctx, providerID, value)
}

func (p *engineProvider) GatewayForModel(ctx context.Context, modelID string) string {
	return p.eng.GatewayForModel(ctx, modelID)
}

func (p *engineProvider) CanonicalModel(ctx context.Context, modelID string) string {
	return p.eng.CanonicalModel(ctx, modelID)
}

func (p *engineProvider) DeploymentRoutingEnabled(override *bool) bool {
	return p.eng.DeploymentRoutingEnabled(override)
}

func (p *engineProvider) DeploymentStatus(ctx context.Context, activeModel string) (string, error) {
	return p.eng.DeploymentStatus(ctx, activeModel)
}

func (p *engineProvider) DeploymentSummary(ctx context.Context, activeModel string) (eyrieengine.DeploymentSummary, error) {
	return p.eng.DeploymentSummary(ctx, activeModel)
}

func (p *engineProvider) RoutingPreview(ctx context.Context, modelID string) (string, error) {
	return p.eng.RoutingPreview(ctx, modelID)
}

func (p *engineProvider) CatalogHealth(ctx context.Context) eyrieengine.CatalogHealth {
	return p.eng.CatalogHealth(ctx)
}

func (p *engineProvider) PreflightWithOptions(ctx context.Context, opts eyrieengine.PreflightOptions) eyrieengine.PreflightReport {
	return p.eng.PreflightWithOptions(ctx, opts)
}

func (p *engineProvider) ActiveSelection(ctx context.Context) eyrieengine.Route {
	return p.eng.ActiveSelection(ctx)
}

func (p *engineProvider) EffectiveSelection(ctx context.Context, opts eyrieengine.SelectionOptions) eyrieengine.Selection {
	return p.eng.EffectiveSelection(ctx, opts)
}

func (p *engineProvider) SetActiveProvider(ctx context.Context, provider string) error {
	return p.eng.SetActiveProvider(ctx, provider)
}

func (p *engineProvider) SetActiveModel(ctx context.Context, modelID string) error {
	return p.eng.SetActiveModel(ctx, modelID)
}

func (p *engineProvider) SetSelection(ctx context.Context, provider, modelID string) error {
	return p.eng.SetSelection(ctx, provider, modelID)
}

func (p *engineProvider) ClearSelection(ctx context.Context) error {
	return p.eng.ClearSelection(ctx)
}

func (p *engineProvider) ProviderStateSecurityStatus() eyrieengine.ProviderStateSecurity {
	return p.eng.ProviderStateSecurityStatus()
}

func (p *engineProvider) MigrateProviderSecrets() error {
	return p.eng.MigrateProviderSecrets()
}

func (p *engineProvider) SupportsNativeCompaction(ctx context.Context, provider, model string) bool {
	return p.eng.SupportsNativeCompaction(ctx, provider, model)
}

func (p *engineProvider) CompactNative(ctx context.Context, req eyrieengine.NativeCompactionRequest) (string, error) {
	return p.eng.CompactNative(ctx, req)
}

// translateProvider bridges the hawk-owned ChatClient port to the Generator and
// NativeCompactor roles. It needs no other Provider facet. The type conversions
// here (internal/types <-> eyrieengine.*) are the single, centralized
// translation point — Hawk's conversation DTOs never leak past it.
type translateProvider struct {
	generator Generator
	compactor NativeCompactor
}

func newChatClientProvider(provider Provider) *translateProvider {
	return &translateProvider{generator: provider, compactor: provider}
}

func (c *translateProvider) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	response, err := c.generator.Generate(ctx, toEngineRequest(messages, opts, types.ContinuationConfig{}))
	if err != nil {
		return nil, err
	}
	return fromEngineResponse(response), nil
}

func (c *translateProvider) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, continuation types.ContinuationConfig) (*types.StreamResult, error) {
	request := toEngineRequest(messages, opts, continuation)
	request.Requirements.Streaming = true
	stream, err := c.generator.Stream(ctx, request)
	if err != nil {
		return nil, err
	}
	events := make(chan types.EyrieStreamEvent, 64)
	streamCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(events)
		defer func() { _ = stream.Close() }()
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
	return llm.NewStreamResult(events, "", closeFn), nil
}

// ManagesResilience tells Hawk not to add provider retry, continuation, or
// protocol-recovery layers around Eyrie's routed transport.
func (c *translateProvider) ManagesResilience() bool { return true }

// NativeCompaction reports whether the bound compactor supports provider-native
// compaction for a provider/model pair.
func (c *translateProvider) NativeCompaction(ctx context.Context, provider, model string) bool {
	if c.compactor == nil {
		return false
	}
	return c.compactor.SupportsNativeCompaction(ctx, provider, model)
}

// CompactNative performs provider-native compaction through the bound compactor.
func (c *translateProvider) CompactNative(ctx context.Context, req eyrieengine.NativeCompactionRequest) (string, error) {
	if c.compactor == nil {
		return "", fmt.Errorf("gateway: no provider")
	}
	return c.compactor.CompactNative(ctx, req)
}

func toEngineRequest(messages []types.EyrieMessage, opts types.ChatOptions, continuation types.ContinuationConfig) eyrieengine.GenerateRequest {
	glmReasoningEnabled := opts.GLMThinkingEnabled != nil && *opts.GLMThinkingEnabled
	request := eyrieengine.GenerateRequest{
		Messages:     toEngineMessages(messages),
		SystemPrompt: opts.System,
		Tools:        toEngineTools(opts.Tools),
		Requirements: eyrieengine.Requirements{
			Streaming:      opts.Stream,
			Tools:          len(opts.Tools) > 0,
			Vision:         messagesContainVision(messages),
			StructuredJSON: opts.ResponseFormat != nil || opts.OutputSchema != "",
			Reasoning:      opts.ReasoningEffort != "" || opts.ThinkingBudgetTokens > 0 || opts.ThinkingMode != "" || glmReasoningEnabled,
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
		Metadata:     eyrieengine.Metadata{UserID: opts.MetadataUserID},
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

// ToEngineMessages returns the messages unchanged: hawk, the engine, and the
// client all speak the canonical contract message type, so no per-field
// conversion is needed. It is exposed for the session layer (e.g. native
// compaction), which translates without reaching into the raw engine.
func ToEngineMessages(messages []types.EyrieMessage) []eyrieengine.Message {
	return messages
}

func toEngineMessages(messages []types.EyrieMessage) []eyrieengine.Message {
	return messages
}

func toEngineTools(tools []types.EyrieTool) []eyrieengine.Tool {
	return tools
}

func toEngineToolChoice(choice *types.ToolChoiceOption) *eyrieengine.ToolChoice {
	return choice
}

func fromEngineResponse(response *eyrieengine.GenerateResponse) *types.EyrieResponse {
	return response
}

func fromEngineEvent(event eyrieengine.Event) (types.EyrieStreamEvent, bool) {
	out := types.EyrieStreamEvent{
		Content: event.Content, Thinking: event.Thinking, RequestID: event.RequestID,
		Usage: event.Usage, StopReason: event.StopReason, TTFTms: event.TTFTms,
	}
	switch event.Type {
	case eyrieengine.EventRouteSelected:
		out.Type = "route_selected"
	case eyrieengine.EventRouteChanged:
		out.Type = "route_changed"
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
		out.TTFT = event.TTFTms
	case eyrieengine.EventDone:
		out.Type = "done"
	case eyrieengine.EventContinuation:
		out.Type = "continuation"
	case eyrieengine.EventWarning:
		out.Type, out.Content = "warning", event.Warning
	default:
		// An unrecognized engine event type means eyrie emits something this
		// adapter has not been taught to translate. Forward it verbatim so no
		// data is silently dropped, but log it so the mapping gap is visible.
		slog.Warn("gateway: forwarding unrecognized engine event type", "type", event.Type)
		out.Type = event.Type
	}
	if event.ToolCall != nil {
		out.ToolCall = event.ToolCall
	}
	if event.Route != nil {
		out.Route = event.Route
	}
	return out, true
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
