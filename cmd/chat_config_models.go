package cmd

import (
	"context"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/runtime"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

// configModelOption is one row in the /config model picker (display from eyrie, id for settings).
type configModelOption struct {
	ID               string
	DisplayName      string
	Owner            string
	ContextWindow    int
	InputPricePer1M  float64
	OutputPricePer1M float64
}

var (
	modelCache   = make(map[string][]configModelOption)
	modelCacheMu sync.RWMutex
)

// InvalidateModelCache clears all in-memory model picker rows.
func InvalidateModelCache() {
	modelCacheMu.Lock()
	modelCache = make(map[string][]configModelOption)
	modelCacheMu.Unlock()
	hawkconfig.InvalidateConfigUICache()
}

// InvalidateModelCacheProvider drops one gateway's cached picker rows.
func InvalidateModelCacheProvider(provider string) {
	modelCacheMu.Lock()
	delete(modelCache, strings.TrimSpace(provider))
	modelCacheMu.Unlock()
	hawkconfig.InvalidateConfigUICache()
}

func fetchModelsAsync(provider string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		provider = strings.TrimSpace(provider)
		if provider == "" {
			provider = hawkconfig.DefaultModelProviderFilter(ctx)
		}
		entries, err := runtime.ListModels(ctx, runtime.ListModelsOpts{ProviderID: provider, Source: runtime.ListSourceAuto})
		if err != nil {
			if _, derr := runtime.Discover(ctx); derr == nil {
				InvalidateModelCacheProvider(provider)
				entries, err = runtime.ListModels(ctx, runtime.ListModelsOpts{ProviderID: provider, Source: runtime.ListSourceAuto})
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

func configModelOptionsFromEyrie(entries []runtime.ModelEntry) []configModelOption {
	opts := make([]configModelOption, len(entries))
	for i, e := range entries {
		opts[i] = configModelOption{
			ID:               e.ID,
			DisplayName:      catalog.DisplayModelLabel(e.ID, e.DisplayName),
			Owner:            catalog.DisplayModelOwner(e.Owner, e.ID),
			ContextWindow:    e.ContextWindow,
			InputPricePer1M:  e.InputPricePer1M,
			OutputPricePer1M: e.OutputPricePer1M,
		}
	}
	return opts
}

func configModelOptionsFromCatalog(entries []catalog.ModelCatalogEntry) []configModelOption {
	opts := make([]configModelOption, len(entries))
	for i, e := range entries {
		opts[i] = configModelOption{
			ID:               e.ID,
			DisplayName:      catalog.DisplayModelLabel(e.ID, e.DisplayName),
			Owner:            catalog.DisplayModelOwner(e.Owner, e.ID, e.LiveMetadata),
			ContextWindow:    e.ContextWindow,
			InputPricePer1M:  e.InputPricePer1M,
			OutputPricePer1M: e.OutputPricePer1M,
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
	if compiled := hawkconfig.CompiledCatalogV1(); compiled != nil {
		entries := catalog.ModelEntriesForProvider(compiled, provider)
		if len(entries) > 0 {
			opts := configModelOptionsFromCatalog(entries)
			modelCacheMu.Lock()
			modelCache[provider] = opts
			modelCacheMu.Unlock()
			return opts
		}
	}
	return nil
}
