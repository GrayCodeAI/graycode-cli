package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// ApplyEyrieCredentials discovers catalog and syncs provider routing.
func ApplyEyrieCredentials(ctx context.Context) (*runtime.ApplyResult, error) {
	return ApplyCredentials(ctx)
}

// OptionsFromSetupUI converts setup UI to hawk model options.
func OptionsFromSetupUI(result *runtime.ApplyResult, providerFilter string) []ModelOption {
	if result == nil || result.Setup == nil {
		return nil
	}
	var out []ModelOption
	for _, p := range result.Setup.Providers {
		if providerFilter != "" && p.ID != providerFilter {
			continue
		}
		for _, m := range p.Models {
			out = append(out, ModelOption{ID: m.CanonicalID, DisplayName: m.DisplayName})
		}
	}
	return out
}
