package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// ListModelSource re-exported from runtime.
type ListModelSource = runtime.ListModelSource

const (
	ListSourceAuto  = runtime.ListSourceAuto
	ListSourceCache = runtime.ListSourceCache
	ListSourceLive  = runtime.ListSourceLive
)

// ListModelsOpts configures unified model listing.
type ListModelsOpts = runtime.ListModelsOpts

// ModelEntry is one model row for hawk pickers.
type ModelEntry = runtime.ModelEntry

// ListModels returns models using registry-driven live vs cache selection.
func ListModels(ctx context.Context, opts ListModelsOpts) ([]ModelEntry, error) {
	return runtime.ListModels(ctx, opts)
}

// ListModelsForProvider lists models with auto source selection.
func ListModelsForProvider(ctx context.Context, providerID string) ([]ModelEntry, error) {
	return runtime.ListModels(ctx, ListModelsOpts{
		ProviderID: providerID,
		Source:     ListSourceAuto,
	})
}

// FormatSetupError maps setup failures to user-facing messages.
func FormatSetupError(providerID string, err error) string {
	if err == nil {
		return ""
	}
	if formatted := runtime.FormatSetupError(providerID, err); formatted != nil {
		return formatted.Error()
	}
	return err.Error()
}

// LocalCredentialInference returns metadata for no-key providers.
func LocalCredentialInference(providerID string) (runtime.CredentialInference, error) {
	return runtime.LocalCredentialInference(providerID)
}

// ProviderSetupOption is one /config hub row.
type ProviderSetupOption = runtime.ProviderSetupOption

// ListProviderSetupOptions returns dynamic hub options from eyrie.
func ListProviderSetupOptions(ctx context.Context) []ProviderSetupOption {
	return runtime.ListProviderSetupOptions(ctx)
}

// ModelOption is a simplified picker row for hawk config.
type ModelOption struct {
	ID               string
	DisplayName      string
	Owner            string
	ContextWindow    int
	InputPricePer1M  float64
	OutputPricePer1M float64
}

// ModelOptionsFromEntries converts runtime entries to hawk picker rows.
func ModelOptionsFromEntries(in []ModelEntry) []ModelOption {
	out := make([]ModelOption, len(in))
	for i, e := range in {
		out[i] = ModelOption{
			ID:               e.ID,
			DisplayName:      e.DisplayName,
			Owner:            e.Owner,
			ContextWindow:    e.ContextWindow,
			InputPricePer1M:  e.InputPricePer1M,
			OutputPricePer1M: e.OutputPricePer1M,
		}
	}
	return out
}
