package config

import (
	"context"
	"strings"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

// ActiveModel returns the selected model from eyrie provider.json (not hawk settings).
func ActiveModel(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	return engine.ActiveSelection(ctx).Model
}

// ActiveProvider returns the selected provider from eyrie provider.json.
func ActiveProvider(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := newEyrieEngine()
	if err != nil {
		return ""
	}
	return engine.ActiveSelection(ctx).Provider
}

// ActiveProviderID canonicalizes a host-facing provider/gateway id through Eyrie runtime.
func ActiveProviderID(provider string) string {
	return eyrieengine.NormalizeProviderID(provider)
}

// SetActiveModel persists model selection to eyrie provider.json.
func SetActiveModel(ctx context.Context, modelID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	return engine.SetActiveModel(ctx, modelID)
}

// SetActiveProvider persists provider selection to eyrie provider.json.
func SetActiveProvider(ctx context.Context, provider string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	return engine.SetActiveProvider(ctx, provider)
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
