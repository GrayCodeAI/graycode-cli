package cmd

import (
	"context"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// configModelOption is one row in the /config model picker (display from eyrie, id for settings).
type configModelOption struct {
	ID               string
	CanonicalID      string
	ProviderID       string
	GatewayID        string
	DisplayName      string
	Owner            string
	ContextWindow    int
	InputPricePer1M  float64
	OutputPricePer1M float64
	PriceKnown       bool
	Capabilities     []string
}

var (
	modelCache         = make(map[string][]configModelOption)
	modelCacheMu       sync.RWMutex
	modelSyncAttempted = make(map[string]bool)
	modelSyncMu        sync.Mutex
)

// InvalidateModelCache clears all in-memory model picker rows.
func InvalidateModelCache() {
	modelCacheMu.Lock()
	modelCache = make(map[string][]configModelOption)
	modelCacheMu.Unlock()
	modelSyncMu.Lock()
	modelSyncAttempted = make(map[string]bool)
	modelSyncMu.Unlock()
	invalidatePlatformContextCache()
	graycodeconfig.InvalidateConfigUICache()
}

// InvalidateModelCacheProvider drops one gateway's cached picker rows.
func InvalidateModelCacheProvider(provider string) {
	provider = strings.TrimSpace(provider)
	modelCacheMu.Lock()
	delete(modelCache, provider)
	modelCacheMu.Unlock()
	modelSyncMu.Lock()
	delete(modelSyncAttempted, provider)
	modelSyncMu.Unlock()
	graycodeconfig.InvalidateConfigUICache()
}

func fetchModelsAsync(provider string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		provider = strings.TrimSpace(provider)
		if provider == "" {
			provider = graycodeconfig.DefaultModelProviderFilter(ctx)
		}
		entries, err := graycodeconfig.ListEngineModels(ctx, provider, false)
		if err != nil {
			if _, derr := graycodeconfig.ListEngineModels(ctx, provider, true); derr == nil {
				InvalidateModelCacheProvider(provider)
				entries, err = graycodeconfig.ListEngineModels(ctx, provider, false)
			}
		}
		if err != nil {
			return modelsFetchedMsg{provider: provider, err: err}
		}
		opts := configModelOptionsFromEyrie(entries)
		if len(opts) > 0 {
			modelCacheMu.Lock()
			modelCache[provider] = opts
			modelCacheMu.Unlock()
		}
		return modelsFetchedMsg{options: opts, provider: provider}
	}
}

func configModelOptionsFromEyrie(entries []graycodeconfig.EngineModel) []configModelOption {
	opts := make([]configModelOption, len(entries))
	for i, e := range entries {
		opts[i] = configModelOption{
			ID:               e.ID,
			CanonicalID:      e.CanonicalID,
			ProviderID:       e.ProviderID,
			GatewayID:        e.GatewayID,
			DisplayName:      e.DisplayName,
			Owner:            e.Owner,
			ContextWindow:    e.ContextWindow,
			InputPricePer1M:  e.InputPricePer1M,
			OutputPricePer1M: e.OutputPricePer1M,
			PriceKnown:       e.PriceKnown,
			Capabilities:     append([]string(nil), e.Capabilities...),
		}
	}
	return opts
}

func filterConfigModelOptions(opts []configModelOption, query string) []configModelOption {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return opts
	}
	out := make([]configModelOption, 0, len(opts))
	for _, opt := range opts {
		if modelOptionMatchesQuery(opt, query) {
			out = append(out, opt)
		}
	}
	return out
}

func modelOptionMatchesQuery(opt configModelOption, query string) bool {
	candidates := []string{
		strings.ToLower(strings.TrimSpace(opt.ID)),
		strings.ToLower(strings.TrimSpace(opt.DisplayName)),
		strings.ToLower(strings.TrimSpace(opt.Owner)),
		strings.ToLower(shortModelID(opt.ID)),
		strings.ToLower(strings.Join(opt.Capabilities, " ")),
	}
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(candidate, query) {
			return true
		}
	}
	return false
}

func (m chatModel) configModelSearchQuery() string {
	if m.configModelSearchActive {
		return strings.TrimSpace(m.configInput.Value())
	}
	return strings.TrimSpace(m.configModelSearch)
}

func (m chatModel) configFilteredModelOptions() []configModelOption {
	return filterConfigModelOptions(m.configModelOptions, m.configModelSearchQuery())
}

// ensureModelCacheLoaded tries a one-time live ListModels when the in-memory cache is cold.
// Footer context (e.g. 1.0m for mimo-v2.5-pro) comes from the platform API merge, not the 128k default.
func ensureModelCacheLoaded(provider string) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return
	}
	modelCacheMu.RLock()
	if cached, ok := modelCache[provider]; ok && len(cached) > 0 {
		modelCacheMu.RUnlock()
		return
	}
	modelCacheMu.RUnlock()

	modelSyncMu.Lock()
	if modelSyncAttempted[provider] {
		modelSyncMu.Unlock()
		return
	}
	modelSyncAttempted[provider] = true
	modelSyncMu.Unlock()

	ctx := context.Background()
	entries, err := graycodeconfig.ListEngineModels(ctx, provider, false)
	if err != nil {
		if _, derr := graycodeconfig.ListEngineModels(ctx, provider, true); derr == nil {
			entries, err = graycodeconfig.ListEngineModels(ctx, provider, false)
		}
	}
	if err != nil || len(entries) == 0 {
		modelSyncMu.Lock()
		delete(modelSyncAttempted, provider)
		modelSyncMu.Unlock()
		return
	}
	opts := configModelOptionsFromEyrie(entries)
	modelCacheMu.Lock()
	modelCache[provider] = opts
	modelCacheMu.Unlock()
}

// lookupModelOption returns live-cached catalog metadata for a provider/model pair.
func lookupModelOption(provider, modelID string) (configModelOption, bool) {
	provider = strings.TrimSpace(provider)
	modelID = strings.TrimSpace(modelID)
	if provider == "" || modelID == "" {
		return configModelOption{}, false
	}
	ensureModelCacheLoaded(provider)
	for _, o := range loadConfigModelOptions(provider) {
		if o.ID == modelID {
			return o, true
		}
	}
	return configModelOption{}, false
}

// applyModelOptionToSession copies live context window (and pricing when known) into the session.
func applyModelOptionToSession(sess *engine.Session, opt configModelOption) {
	if sess == nil {
		return
	}
	if opt.ContextWindow > 0 {
		if persist := sess.Persistence(); persist != nil {
			persist.SetContextWindowCached(opt.ContextWindow)
		}
		sess.EnsureAutoCompactor()
	}
	if opt.PriceKnown {
		engine.RegisterLivePricing(opt.ID, opt.InputPricePer1M, opt.OutputPricePer1M)
	}
}

func applyLiveModelMetadata(sess *engine.Session, provider, modelID string) {
	if opt, ok := lookupModelOption(provider, modelID); ok {
		applyModelOptionToSession(sess, opt)
		cw := 0
		if persist := sess.Persistence(); persist != nil {
			cw = persist.ContextWindowCached()
		}
		if sess != nil && cw > 0 {
			return
		}
	}
	if isXiaomiMimoProvider(provider) {
		if w := platformContextForNativeModel(modelID); w > 0 {
			applyModelOptionToSession(sess, configModelOption{
				ID:            modelID,
				ContextWindow: w,
			})
		}
	}
}

func loadConfigModelOptions(provider string) []configModelOption {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
	}
	modelCacheMu.RLock()
	if cached, ok := modelCache[provider]; ok && len(cached) > 0 {
		modelCacheMu.RUnlock()
		return cached
	}
	modelCacheMu.RUnlock()
	entries, err := graycodeconfig.ListEngineModels(context.Background(), provider, false)
	if err == nil && len(entries) > 0 {
		opts := configModelOptionsFromEyrie(entries)
		modelCacheMu.Lock()
		modelCache[provider] = opts
		modelCacheMu.Unlock()
		return opts
	}
	return nil
}
