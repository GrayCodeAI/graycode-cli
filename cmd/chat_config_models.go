package cmd

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

// configModelOption is one row in the /config model picker (display from eyrie, id for settings).
type configModelOption struct {
	ID          string
	DisplayName string
}

var modelCache = make(map[string][]configModelOption)

// InvalidateModelCache clears in-memory model picker rows (call after credential apply or catalog refresh).
func InvalidateModelCache() {
	modelCache = make(map[string][]configModelOption)
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
				InvalidateModelCache()
				entries, err = eyrieclient.ListModelsForProvider(ctx, provider)
			}
		}
		if err != nil {
			return modelsFetchedMsg{provider: provider, err: err}
		}
		opts := configModelOptionsFromEyrie(entries)
		if len(opts) > 0 {
			modelCache[provider] = opts
		}
		return modelsFetchedMsg{options: opts, provider: provider}
	}
}

func configModelOptionsFromEyrie(entries []eyrieclient.ModelEntry) []configModelOption {
	out := eyrieclient.ModelOptionsFromEntries(entries)
	opts := make([]configModelOption, len(out))
	for i, o := range out {
		opts[i] = configModelOption{ID: o.ID, DisplayName: o.DisplayName}
	}
	return opts
}

func loadConfigModelOptions(provider string) []configModelOption {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
	}
	if cached, ok := modelCache[provider]; ok && len(cached) > 0 {
		return cached
	}
	entries, err := eyrieclient.ListModelsForProvider(context.Background(), provider)
	if err != nil || len(entries) == 0 {
		return nil
	}
	opts := configModelOptionsFromEyrie(entries)
	if len(opts) > 0 {
		modelCache[provider] = opts
	}
	return opts
}
