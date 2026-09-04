package config

import (
	"context"
)

// DefaultModelProviderFilter picks which graycode-router provider to list models for when the UI
// has no explicit filter. Host prefs (settings) win; otherwise graycode-router routing/deployments decide.
func DefaultModelProviderFilter(ctx context.Context) string {
	if p := ActiveGateway(ctx); p != "" {
		return p
	}
	engine, err := newGraycodeRouterEngine()
	if err != nil {
		return ""
	}
	return engine.DefaultProviderFilter(ctx)
}
