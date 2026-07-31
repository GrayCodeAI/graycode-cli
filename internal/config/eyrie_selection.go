package config

import (
	"context"
	"strings"

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

// SetActiveSelection validates and persists a provider/model pair atomically.
// Use this for migrations and other flows where persisting only one half would
// leave provider state inconsistent.
func SetActiveSelection(ctx context.Context, provider, modelID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	return engine.SetSelection(ctx, provider, modelID)
}

// migrateStoredModelProvider moves model/provider from ~/.hawk/settings.json into eyrie once.
func migrateStoredModelProvider(s *Settings) {
	if s == nil {
		return
	}
	ctx := context.Background()
	oldModel := strings.TrimSpace(s.Model)
	oldProvider := strings.TrimSpace(s.Provider)
	activeModel := strings.TrimSpace(ActiveModel(ctx))
	activeProvider := strings.TrimSpace(ActiveProvider(ctx))
	changed := false

	// Existing Eyrie state is authoritative. Otherwise migrate a stored pair
	// in one validated write so a rejected model cannot strand only the
	// provider in the destination or silently erase the user's source value.
	if activeModel != "" {
		if oldModel != "" {
			s.Model = ""
			changed = true
		}
		if oldProvider != "" {
			s.Provider = ""
			changed = true
		}
	} else if oldModel != "" {
		provider := activeProvider
		if provider == "" {
			provider = oldProvider
		}
		if err := SetActiveSelection(ctx, provider, oldModel); err == nil {
			s.Model = ""
			s.Provider = ""
			changed = true
		}
	} else if oldProvider != "" {
		if activeProvider != "" || SetActiveProvider(ctx, oldProvider) == nil {
			s.Provider = ""
			changed = true
		}
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
