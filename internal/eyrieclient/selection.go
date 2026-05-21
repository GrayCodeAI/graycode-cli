package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// ActiveModel returns the model selected in eyrie provider.json.
func ActiveModel(ctx context.Context) string {
	return runtime.ActiveModel(ctx)
}

// ActiveProvider returns the provider selected in eyrie provider.json.
func ActiveProvider(ctx context.Context) string {
	return runtime.ActiveProvider(ctx)
}

// SetActiveModel saves the user's model choice to eyrie (provider.json).
func SetActiveModel(ctx context.Context, modelID string) error {
	return runtime.SetActiveModel(ctx, modelID)
}

// SetActiveProvider saves the active provider to eyrie (provider.json).
func SetActiveProvider(ctx context.Context, provider string) error {
	return runtime.SetActiveProvider(ctx, provider)
}
