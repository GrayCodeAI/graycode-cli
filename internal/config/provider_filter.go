package config

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// DefaultModelProviderFilter picks which eyrie provider to list models for when the UI
// has no explicit filter. Host prefs (settings) win; otherwise eyrie routing/deployments decide.
func DefaultModelProviderFilter(ctx context.Context) string {
	if p := catalogProviderID(ActiveProvider(ctx)); p != "" {
		return p
	}
	if m := strings.TrimSpace(ActiveModel(ctx)); m != "" {
		if p := ProviderOfModel(m); p != "" {
			return catalogProviderID(p)
		}
	}
	return runtime.DefaultModelProviderFilter(ctx)
}
