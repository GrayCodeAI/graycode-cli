package config

import (
	"context"
	"strings"
)

// ResolveCanonicalModel maps aliases and native IDs to catalog canonical model IDs.
func ResolveCanonicalModel(model string) string {
	return CanonicalModelID(context.Background(), strings.TrimSpace(model))
}

// DeploymentStatusReport returns graycode deployment routing diagnostics.
func DeploymentStatusReport(ctx context.Context, activeModel string) (string, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return "", err
	}
	return engine.DeploymentStatus(ctx, activeModel)
}

func DeploymentStatusReportWithSettings(ctx context.Context, settings Settings, activeModel string) (string, error) {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return "", err
	}
	return engine.DeploymentStatus(ctx, activeModel)
}

// RoutingPreviewJSON returns effective routing for a model (eyrie routing JSON preview).
func RoutingPreviewJSON(ctx context.Context, model string) (string, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return "", err
	}
	return engine.RoutingPreview(ctx, model)
}

func RoutingPreviewJSONWithSettings(ctx context.Context, settings Settings, model string) (string, error) {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return "", err
	}
	return engine.RoutingPreview(ctx, model)
}
