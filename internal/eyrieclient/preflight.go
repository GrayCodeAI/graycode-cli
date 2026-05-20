package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// PreflightReport re-export.
type PreflightReport = runtime.PreflightReport

// PreflightCheck re-export.
type PreflightCheck = runtime.PreflightCheck

// Preflight evaluates readiness to chat (catalog, credentials, model, live models).
func Preflight(ctx context.Context) PreflightReport {
	return runtime.Preflight(ctx)
}

// FormatPreflightReport formats preflight for CLI output.
func FormatPreflightReport(r PreflightReport) string {
	return runtime.FormatPreflightReport(r)
}

// ListModelsForProviderLive lists models directly from provider APIs (bypasses cache).
func ListModelsForProviderLive(ctx context.Context, providerID string) ([]ModelEntry, error) {
	return runtime.ListModels(ctx, runtime.ListModelsOpts{
		ProviderID: providerID,
		Source:     runtime.ListSourceLive,
	})
}

// ListModelsForProviderAfterApply lists models after credential apply (cache + live fallback).
func ListModelsForProviderAfterApply(ctx context.Context, providerID string) ([]ModelEntry, error) {
	entries, err := runtime.ListModels(ctx, runtime.ListModelsOpts{
		ProviderID: providerID,
		Source:     runtime.ListSourceLive,
	})
	if err == nil && len(entries) > 0 {
		return entries, nil
	}
	return runtime.ListModels(ctx, runtime.ListModelsOpts{
		ProviderID: providerID,
		Source:     runtime.ListSourceAuto,
	})
}
