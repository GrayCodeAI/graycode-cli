package config

import (
	"context"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

type EngineModel = eyrieengine.Model

// newEyrieEngine is Hawk's composition root for Eyrie's stable host facade.
// Eyrie retains ownership of paths and the platform secret store through its
// backward-compatible defaults during migration. Tests may continue injecting
// lower-level stores through Eyrie's existing seams.
func newEyrieEngine() (*eyrieengine.Engine, error) {
	return eyrieengine.New(eyrieengine.Options{})
}

// ListEngineModels returns model-picker rows through Eyrie's stable facade.
func ListEngineModels(ctx context.Context, providerID string, refresh bool) ([]EngineModel, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil, err
	}
	return engine.ListModels(ctx, providerID, refresh)
}
