package config

import (
	"context"
)

// DefaultModelProviderFilter picks which eyrie provider to list models for when the UI
// has no explicit filter. Host prefs (settings) win; otherwise eyrie routing/deployments decide.
func DefaultModelProviderFilter(ctx context.Context) string {
	if p := ActiveGateway(ctx); p != "" {
		return p
	}
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	return engine.DefaultProviderFilter(ctx)
}
