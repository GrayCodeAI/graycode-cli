package config

import (
	"context"
	"os"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// ResolveCanonicalModel maps aliases and native IDs to catalog canonical model IDs.
func ResolveCanonicalModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	compiled, err := loadEyrieCatalogV1(context.Background(), false)
	if err != nil || compiled == nil {
		return model
	}
	if canonical, ok := compiled.CanonicalModelForAliasOrID(model); ok {
		return canonical
	}
	if strings.Contains(model, "/") {
		return model
	}
	return model
}

// DeploymentStatusReport returns hawk deployment routing diagnostics.
func DeploymentStatusReport(ctx context.Context, activeModel string) (string, error) {
	report, err := setup.DeploymentStatus(ctx, activeModel)
	if err != nil {
		return "", err
	}
	return setup.FormatStatus(report), nil
}

// RoutingPreviewJSON returns effective routing for a model (eyrie routing JSON preview).
func RoutingPreviewJSON(ctx context.Context, model string) (string, error) {
	return setup.RoutingPreview(ctx, model)
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
