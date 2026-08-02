// Package gateway is Hawk's single boundary to Eyrie's provider runtime. It is
// the only package that imports Eyrie; everything else speaks the hawk-owned
// Provider interface and the internal/types DTOs.
//
// hawk = product face (UX/agent/sessions) · eyrie = provider engine
// One-way dependency only: eyrie never imports hawk. See README ecosystems.
package gateway

import (
	"context"
	"log/slog"
	"sync"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// Gateway is Hawk's single boundary to the Eyrie provider runtime. It embeds
// Provider so every engine method is forwarded, and it is the only type that
// constructs one (via New). All other Hawk packages hold a *Gateway or speak
// the Provider interface — never an *eyrieengine.Engine.
//
// Construction is centralized here: New is the only call to eyrieengine.New and
// the place Hawk declares its identity to the credential store.
// Gateway is Hawk's single boundary to the Eyrie provider runtime. It embeds the
// Provider roles so every engine method is forwarded, and it is the only type
// that constructs one (via New). All other Hawk packages hold a *Gateway or speak
// the Provider interface — never an *eyrieengine.Engine. *Gateway satisfies the
// composite Provider interface.
type Gateway struct {
	Generator
	NativeCompactor
	ModelCatalog
	CredentialManager
	SelectionManager
	GatewayInspector
	CatalogMaintenance
}

// declareHawkIdentity sets Eyrie's OS keychain service name to "hawk" so existing
// credentials (filed under "hawk") stay readable under Eyrie's now host-neutral
// default. It is idempotent and runs exactly once. Called from New so the
// identity is always declared before any credential read, no matter which New
// path runs first.
var declareHawkIdentity = sync.OnceFunc(func() {
	eyrieengine.SetSecretStoreServiceName("hawk")
})

// New composes the Eyrie engine for one effective settings snapshot and wraps it
// as a Provider. It is the single composition root — every eyrieengine.New call
// in Hawk flows through here.
func New(ctx context.Context, providers []CustomProviderConfig) (*Gateway, error) {
	// Declare hawk's identity to the credential store FIRST, before
	// constructing the engine, so no credential read ever happens under
	// Eyrie's host-neutral default service name. The OnceFunc makes this
	// safe to call from every construction path.
	declareHawkIdentity()

	gateways := customGatewaysFromSettings(providers)
	eng, err := eyrieengine.New(eyrieengine.Options{CustomGateways: gateways})
	if err != nil {
		return nil, err
	}
	p := newEngineProvider(eng)
	return &Gateway{
		Generator:          p,
		NativeCompactor:    p,
		ModelCatalog:       p,
		CredentialManager:  p,
		SelectionManager:   p,
		GatewayInspector:   p,
		CatalogMaintenance: p,
	}, nil
}

// BuildCustomGateways maps Hawk's OpenAI-compatible provider config onto
// Eyrie's CustomGateway spec. Shared by gateway.New, config.eyrie_engine, and
// engine.session_factory so a new CustomProviderConfig field only needs wiring
// in one place.
func BuildCustomGateways(providers []CustomProviderConfig) []eyrieengine.CustomGateway {
	gateways := make([]eyrieengine.CustomGateway, 0, len(providers))
	for _, provider := range providers {
		if provider.Name == "" && provider.BaseURL == "" {
			continue
		}
		gateways = append(gateways, eyrieengine.CustomGateway{
			ID: provider.Name, BaseURL: provider.BaseURL,
			CredentialEnv: provider.APIKeyEnv, DefaultModel: provider.Model,
		})
	}
	return gateways
}

// customGatewaysFromSettings is the internal alias kept for backward compat.
func customGatewaysFromSettings(providers []CustomProviderConfig) []eyrieengine.CustomGateway {
	return BuildCustomGateways(providers)
}

// CustomProviderConfig is Hawk's spec for a user-defined OpenAI-compatible
// provider. Kept here (rather than reusing config.CustomProviderConfig) so the
// gateway package does not import config and create an import cycle.
type CustomProviderConfig struct {
	Name      string
	BaseURL   string
	APIKeyEnv string
	Model     string
}

// ModelInfo is Hawk's product-facing view of Eyrie model metadata.
type ModelInfo struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ContextSize int     `json:"context_size"`
	InputPrice  float64 `json:"input_price_per_million"`
	OutputPrice float64 `json:"output_price_per_million"`
	Description string  `json:"description,omitempty"`
	Recommended bool    `json:"recommended,omitempty"`
}

func fromEngineModel(model eyrieengine.Model) ModelInfo {
	return ModelInfo{
		Name: model.ID, Provider: model.ProviderID,
		ContextSize: model.ContextWindow,
		InputPrice:  model.InputPricePer1M, OutputPrice: model.OutputPricePer1M,
		Description: model.Description,
	}
}

// ChatClient returns a hawk ChatClient bound to this gateway's Provider.
func (g *Gateway) ChatClient() *translateProvider {
	return newChatClientProvider(g)
}

// MustSelectProvider returns the Provider, or nil if the gateway is unset.
func (g *Gateway) MustSelectProvider() Provider {
	if g == nil {
		return nil
	}
	return g
}

// NewFromEngine wraps an existing *eyrieengine.Engine as a Gateway. Tests that
// inject an Eyrie SecretStore (e.g. compaction-support detection) use it so the
// rest of Hawk still speaks the Gateway boundary.
func NewFromEngine(eng *eyrieengine.Engine) *Gateway {
	if eng == nil {
		return nil
	}
	p := newEngineProvider(eng)
	return &Gateway{
		Generator:          p,
		NativeCompactor:    p,
		ModelCatalog:       p,
		CredentialManager:  p,
		SelectionManager:   p,
		GatewayInspector:   p,
		CatalogMaintenance: p,
	}
}

// --- Stateless package-level lookups -------------------------------------
// These delegate Eyrie reads to one shared default gateway so hawk-owned
// policy packages (routing, config) never import Eyrie themselves. eyrie's
// Engine reloads its catalog and provider config from disk on every method
// call, so a single long-lived gateway returns identical freshness to
// constructing one per call — this just avoids redundant construction. It
// mirrors the sync.Once singleton the old routing code used.
//
// The ctx parameter is retained (though unused here) to keep this helper's
// signature stable across its 11 call sites; ctx still flows through to every
// data call via the helpers below.

var (
	defaultGatewayMu  sync.Mutex
	defaultGatewayVal *Gateway
	// newGatewayFn is the constructor used by defaultGateway. Indirect so
	// tests can inject failure and assert the retry behavior (H8).
	newGatewayFn = func(ctx context.Context) (*Gateway, error) {
		return New(ctx, nil)
	}
)

func defaultGateway(ctx context.Context) *Gateway {
	// Fast path: already constructed.
	defaultGatewayMu.Lock()
	defer defaultGatewayMu.Unlock()
	if defaultGatewayVal != nil {
		return defaultGatewayVal
	}
	// Construct on first successful use. If New fails, log the error and
	// return nil; a later call retries instead of being permanently nil
	// (the sync.Once footgun that discarded the error, H8).
	g, err := newGatewayFn(context.Background())
	if err != nil {
		slog.Error("gateway initialization failed", "error", err)
		return nil
	}
	defaultGatewayVal = g
	return defaultGatewayVal
}

// ModelInfoLookup returns a model by id or alias, or false if unknown.
func ModelInfoLookup(ctx context.Context, name string) (ModelInfo, bool) {
	g := defaultGateway(ctx)
	if g == nil {
		return ModelInfo{}, false
	}
	model, ok, err := g.ModelInfo(ctx, name)
	if err != nil || !ok {
		return ModelInfo{}, false
	}
	return fromEngineModel(model), true
}

// ModelsByProvider returns every model served by a provider/gateway.
func ModelsByProvider(ctx context.Context, provider string) ([]ModelInfo, error) {
	g := defaultGateway(ctx)
	if g == nil {
		return nil, nil
	}
	models, err := g.ListModels(ctx, provider, false)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		out = append(out, fromEngineModel(model))
	}
	return out, nil
}

// RecommendedModel returns the catalog default for a provider, flagged as
// recommended.
func RecommendedModel(ctx context.Context, provider string) (ModelInfo, bool) {
	g := defaultGateway(ctx)
	if g == nil {
		return ModelInfo{}, false
	}
	name := g.DefaultModel(ctx, provider, "")
	if name == "" {
		return ModelInfo{}, false
	}
	info, ok := ModelInfoLookup(ctx, name)
	if ok {
		info.Recommended = true
	}
	return info, ok
}

// DefaultModel returns the catalog default model name for a provider.
func DefaultModel(ctx context.Context, provider string) string {
	g := defaultGateway(ctx)
	if g == nil {
		return ""
	}
	return g.DefaultModel(ctx, provider, "")
}

// AllProviders returns the distinct set of providers/gateways in the catalog.
func AllProviders(ctx context.Context) ([]string, error) {
	g := defaultGateway(ctx)
	if g == nil {
		return nil, nil
	}
	return g.ModelProviders(ctx)
}

// ProviderForModel resolves which provider owns a model name.
func ProviderForModel(ctx context.Context, modelName string) string {
	g := defaultGateway(ctx)
	if g == nil {
		return ""
	}
	return g.ProviderForModel(ctx, modelName)
}

// PreferredModel returns Eyrie's tier-preferred model for a provider.
func PreferredModel(ctx context.Context, provider string, class ModelClass, fallback string) string {
	g := defaultGateway(ctx)
	if g == nil {
		return fallback
	}
	return g.PreferredModel(ctx, provider, class, fallback)
}

// PreferredModels returns up to limit tier-preferred models for a provider.
func PreferredModels(ctx context.Context, primaryProvider string, class ModelClass, limit int) []string {
	g := defaultGateway(ctx)
	if g == nil {
		return nil
	}
	return g.PreferredModels(ctx, primaryProvider, class, limit)
}

// ModelClassOf returns the cost tier of a model.
func ModelClassOf(ctx context.Context, modelID string) ModelClass {
	g := defaultGateway(ctx)
	if g == nil {
		return ModelClassBalanced
	}
	return g.ModelClassOf(ctx, modelID)
}

// PrimaryModel returns the catalog-wide primary model.
func PrimaryModel(ctx context.Context) string {
	g := defaultGateway(ctx)
	if g == nil {
		return ""
	}
	return g.PrimaryModel(ctx)
}

// ModelNames returns all model names known to the catalog.
func ModelNames(ctx context.Context) []string {
	g := defaultGateway(ctx)
	if g == nil {
		return nil
	}
	return g.ModelNames(ctx)
}

// --- hawk-owned mirror of Eyrie's ModelClass tier enum -------------------
// Kept here (rather than importing neutral constants) so the boundary stays
// one-way; values match eyrieengine.ModelClass.
type ModelClass = eyrieengine.ModelClass

const (
	ModelClassEconomical = llm.ModelClassEconomical
	ModelClassBalanced   = llm.ModelClassBalanced
	ModelClassPremium    = llm.ModelClassPremium
	CheckFail            = eyrieengine.CheckFail
)

// NormalizeProviderID canonicalizes a host-facing provider/gateway id.
func NormalizeProviderID(id string) string {
	return eyrieengine.NormalizeProviderID(id)
}

// --- Eyrie report/type re-exports config internals consume ----------------
// These alias Eyrie types that a few config-only report paths return. They
// live in gateway (the single Eyrie importer) rather than config.

type (
	PreflightReport         = eyrieengine.PreflightReport
	PreflightOptions        = eyrieengine.PreflightOptions
	ProviderStateSecurity   = eyrieengine.ProviderStateSecurity
	DeploymentSummary       = eyrieengine.DeploymentSummary
	CredentialStorageReport = eyrieengine.CredentialStorageReport
	CredentialStatus        = eyrieengine.CredentialStatus
	CredentialResolution    = eyrieengine.CredentialResolution
	CredentialProvider      = eyrieengine.CredentialProvider
	GatewayDefs             = eyrieengine.Gateway
	CatalogSnapshot         = eyrieengine.CatalogSnapshot
	Model                   = eyrieengine.Model
	StatePaths              = eyrieengine.StatePaths
	SelectionOptions        = eyrieengine.SelectionOptions
	Selection               = eyrieengine.Selection
	NativeCompactionRequest = eyrieengine.NativeCompactionRequest
)

// Package-level Eyrie helpers that config delegates to (gateway stays the only importer).

func PreflightReportWithOptions(ctx context.Context, opts PreflightOptions) PreflightReport {
	return PreflightWithProviders(ctx, nil, opts)
}

// PreflightWithProviders runs preflight against a gateway built from an
// explicit provider list (a settings snapshot's custom gateways), so concurrent
// commands can isolate custom-provider state.
func PreflightWithProviders(ctx context.Context, providers []CustomProviderConfig, opts PreflightOptions) PreflightReport {
	g, err := New(ctx, providers)
	if err != nil {
		return PreflightReport{}
	}
	return g.PreflightWithOptions(ctx, opts)
}

func FormatPreflight(report PreflightReport) string {
	return eyrieengine.FormatPreflight(report)
}

func IsCatalogCacheRequired(err error) bool {
	return eyrieengine.IsCatalogCacheRequired(err)
}

// RegisteredProviderCount exposes Eyrie's first-class provider count through
// Hawk's single provider-runtime boundary. The count derives from Eyrie's
// provider registry, so adding a provider in Eyrie never requires a Hawk edit.
func RegisteredProviderCount() int {
	return eyrieengine.RegisteredGatewayCount()
}

func SecretStoreName() string { return eyrieengine.SecretStoreName() }

func CredentialStorage(ctx context.Context) CredentialStorageReport {
	return eyrieengine.CredentialStorage(ctx)
}

func MigrateEnvFileCredentials(ctx context.Context) (int, error) {
	return eyrieengine.MigrateEnvFileCredentials(ctx)
}

func CredentialGuidance(providerID, secret string) string {
	return eyrieengine.CredentialGuidance(providerID, secret)
}

func FormatSetupError(providerID string, err error) string {
	return eyrieengine.FormatSetupError(providerID, err)
}

// ParseInlineToolCalls extracts inline tool-call markup from model output.
func ParseInlineToolCalls(content string) (string, []eyrieengine.ToolCall) {
	return eyrieengine.ParseInlineToolCalls(content)
}

func DefaultThinkingDisabled(providerID string) bool {
	return eyrieengine.DefaultThinkingDisabled(providerID)
}

func ThinkingToggleSupported(providerID string) bool {
	return eyrieengine.ThinkingToggleSupported(providerID)
}

func (g *Gateway) DefaultThinkingDisabled(providerID string) bool {
	return eyrieengine.DefaultThinkingDisabled(providerID)
}

func (g *Gateway) ThinkingToggleSupported(providerID string) bool {
	return eyrieengine.ThinkingToggleSupported(providerID)
}

// --- Test fixtures -----------------------------------------------------
// Re-exported from engine so hawk tests inject credential fixtures through the
// single gateway+engine boundary. These are thin aliases only.

// SetDefaultStore replaces the process-wide credential store (for tests).
var SetDefaultStore = eyrieengine.SetDefaultStore

// DefaultStore returns the process-wide credential store (for tests).
var DefaultStore = eyrieengine.DefaultStore

// MapStore is the in-memory credential store for tests (alias).
type MapStore = eyrieengine.MapStore

// AccountForEnv returns the keychain account name for an env var.
func AccountForEnv(envVar string) string { return eyrieengine.AccountForEnv(envVar) }

// HasSecret reports whether a secret exists for an env var (for tests).
func HasSecret(ctx context.Context, envKey string) bool { return eyrieengine.HasSecret(ctx, envKey) }
