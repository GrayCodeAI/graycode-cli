package config

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

type (
	Selection        = gateway.Selection
	SelectionOptions = gateway.SelectionOptions
)

// EffectiveSelection resolves persisted selection and optional host overrides
// through Eyrie's host-neutral engine contract.
func EffectiveSelection(ctx context.Context, opts SelectionOptions) Selection {
	engine, err := newEyrieEngine()
	if err != nil {
		return Selection{}
	}
	return engine.EffectiveSelection(ctx, opts)
}

func EffectiveSelectionWithSettings(ctx context.Context, settings Settings, opts SelectionOptions) Selection {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return Selection{}
	}
	return engine.EffectiveSelection(ctx, opts)
}

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
	return gateway.NormalizeProviderID(provider)
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

func stripHostModelSelection(s Settings) Settings {
	s.Model = ""
	s.Provider = ""
	return s
}
