package cmd

import (
	"context"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/eyrie/catalog"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
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
		entries, err := eyrieclient.ListModelsForProvider(ctx, provider)
		if err != nil {
			if _, derr := eyrieclient.Discover(ctx); derr == nil {
				InvalidateModelCacheProvider(provider)
				entries, err = eyrieclient.ListModelsForProvider(ctx, provider)
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

func configModelOptionsFromEyrie(entries []eyrieclient.ModelEntry) []configModelOption {
	out := eyrieclient.ModelOptionsFromEntries(entries)
	opts := make([]configModelOption, len(out))
	for i, o := range out {
		opts[i] = configModelOption{
			ID:               o.ID,
			DisplayName:      o.DisplayName,
			Owner:            o.Owner,
			ContextWindow:    o.ContextWindow,
			InputPricePer1M:  o.InputPricePer1M,
			OutputPricePer1M: o.OutputPricePer1M,
		}
	}
	return opts
}

func configModelOptionsFromCatalog(entries []catalog.ModelCatalogEntry) []configModelOption {
	opts := make([]configModelOption, len(entries))
	for i, e := range entries {
		owner := catalog.ModelOwner(e)
		opts[i] = configModelOption{
			ID:               e.ID,
			DisplayName:      e.DisplayName,
			Owner:            owner,
			ContextWindow:    e.ContextWindow,
			InputPricePer1M:  e.InputPricePer1M,
			OutputPricePer1M: e.OutputPricePer1M,
		}
	}
	return opts
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
