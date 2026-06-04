package config

import (
	"context"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

const ProviderXiaomiTokenPlan = "xiaomi_mimo_token_plan"

// NeedsXiaomiTokenPlanRegion reports whether the Token Plan gateway still needs a cluster pick.
func NeedsXiaomiTokenPlanRegion(providerID string) bool {
	if strings.TrimSpace(providerID) != ProviderXiaomiTokenPlan {
		return false
	}
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return true
	}
	_, err := xiaomi.NormalizeRegion(cfg.XiaomiMimoTokenPlanRegion)
	return err != nil
}

// SetXiaomiTokenPlanRegion persists region (cn, sgp, ams) and syncs env for probe/discovery.
func SetXiaomiTokenPlanRegion(region string) error {
	normalized, err := xiaomi.NormalizeRegion(region)
	if err != nil {
		return err
	}
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		cfg = &eyriecfg.ProviderConfig{}
	}
	cfg.XiaomiMimoTokenPlanRegion = string(normalized)
	if err := eyriecfg.SaveProviderConfig(cfg, ""); err != nil {
		return err
	}
	_ = os.Setenv(eyriecfg.EnvXiaomiTokenPlanRegion, string(normalized))
	base, err := eyriecfg.ResolveXiaomiOpenAIBase(ProviderXiaomiTokenPlan, cfg)
	if err == nil && base != "" {
		_ = os.Setenv(eyriecfg.EnvXiaomiTokenPlanBaseURL, base)
		cfg.XiaomiMimoTokenPlanBaseURL = base
		_ = eyriecfg.SaveProviderConfig(cfg, "")
	}
	return nil
}

// XiaomiTokenPlanRegionLabel returns the saved cluster id for UI (cn, sgp, ams) or "" if unset.
func XiaomiTokenPlanRegionLabel() string {
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return ""
	}
	r, err := xiaomi.NormalizeRegion(cfg.XiaomiMimoTokenPlanRegion)
	if err != nil {
		return ""
	}
	return string(r)
}

// ApplyXiaomiTokenPlanRegionEnv sets process env from provider.json before credential probe.
func ApplyXiaomiTokenPlanRegionEnv(ctx context.Context) {
	_ = ctx
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return
	}
	if r := strings.TrimSpace(cfg.XiaomiMimoTokenPlanRegion); r != "" {
		_ = os.Setenv(eyriecfg.EnvXiaomiTokenPlanRegion, r)
	}
	if base, err := eyriecfg.ResolveXiaomiOpenAIBase(ProviderXiaomiTokenPlan, cfg); err == nil && base != "" {
		_ = os.Setenv(eyriecfg.EnvXiaomiTokenPlanBaseURL, base)
	}
}
