package config

import (
	"context"
	"os"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/runtime"
)

// ResolveCanonicalModel maps aliases and native IDs to catalog canonical model IDs.
func ResolveCanonicalModel(model string) string {
	return runtime.ResolveCanonicalModel(context.Background(), strings.TrimSpace(model))
}

// DeploymentStatusReport returns hawk deployment routing diagnostics.
func DeploymentStatusReport(ctx context.Context, activeModel string) (string, error) {
	report, err := runtime.DeploymentStatus(ctx, activeModel)
	if err != nil {
		return "", err
	}
	return runtime.FormatDeploymentStatus(report), nil
}

// RoutingPreviewJSON returns effective routing for a model (eyrie routing JSON preview).
func RoutingPreviewJSON(ctx context.Context, model string) (string, error) {
	return runtime.RoutingPreview(ctx, model)
}

// MigrateProviderConfig upgrades ~/.hawk/provider.json to deployment v2 in place.
func MigrateProviderConfig() error {
	path := eyriecfg.GetProviderConfigPath()
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	cfg := eyriecfg.LoadProviderConfig("")
	cfg = eyriecfg.EnsureDeploymentConfigV2(cfg)
	if cfg == nil {
		return nil
	}
	return eyriecfg.SaveProviderConfig(cfg, path)
}
