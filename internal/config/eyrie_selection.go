package config

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// ActiveModel returns the selected model from eyrie provider.json (not hawk settings).
func ActiveModel(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtime.ActiveModel(ctx)
}

// ActiveProvider returns the selected provider from eyrie provider.json.
func ActiveProvider(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtime.ActiveProviderID(runtime.ActiveProvider(ctx))
}

// ActiveProviderID canonicalizes a host-facing provider/gateway id through Eyrie runtime.
func ActiveProviderID(provider string) string {
	return runtime.ActiveProviderID(provider)
}

// SetActiveModel persists model selection to eyrie provider.json.
func SetActiveModel(ctx context.Context, modelID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtime.SetActiveModel(ctx, modelID)
}

// SetActiveProvider persists provider selection to eyrie provider.json.
func SetActiveProvider(ctx context.Context, provider string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtime.SetActiveProvider(ctx, runtime.ActiveProviderID(provider))
}

// migrateLegacyModelProvider moves model/provider from ~/.hawk/settings.json into eyrie once.
func migrateLegacyModelProvider(s *Settings) {
	if s == nil {
		return
	}
	ctx := context.Background()
	changed := false
	if m := strings.TrimSpace(s.Model); m != "" {
		if strings.TrimSpace(ActiveModel(ctx)) == "" {
			_ = SetActiveModel(ctx, m)
		}
		s.Model = ""
		changed = true
	}
	if p := strings.TrimSpace(s.Provider); p != "" {
		if strings.TrimSpace(ActiveProvider(ctx)) == "" {
			_ = SetActiveProvider(ctx, p)
		}
		s.Provider = ""
		changed = true
	}
	if changed {
		_ = SaveGlobal(*s)
	}
}

func stripHostModelSelection(s Settings) Settings {
	s.Model = ""
	s.Provider = ""
	return s
}
