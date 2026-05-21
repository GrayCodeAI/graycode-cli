package config

import (
	"context"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// DefaultModelProviderFilter picks which eyrie provider to list models for when the UI
// has no explicit filter. Host prefs (settings) win; otherwise eyrie routing/deployments decide.
func DefaultModelProviderFilter(ctx context.Context) string {
	if p := ActiveGateway(ctx); p != "" {
		return p
	}
	return runtime.DefaultModelProviderFilter(ctx)
}
