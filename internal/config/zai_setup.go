package config

import (
	"context"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/zai"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

const ProviderZAICoding = "z-ai-coding"

// NeedsZAIRegion reports whether the Z.AI gateway (especially Coding Plan) still needs a region pick.
func NeedsZAIRegion(providerID string) bool {
	p := strings.TrimSpace(providerID)
	if p != ProviderZAICoding && p != "z-ai" {
		return false
	}
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return true // first time, offer choice
	}
	region := zaiRegionFromConfig(cfg, p)
	_, err := zai.NormalizeRegion(region)
	return err != nil
}

func zaiRegionFromConfig(cfg *eyriecfg.ProviderConfig, providerID string) string {
	if cfg == nil {
		return ""
	}
	if providerID == ProviderZAICoding {
		return cfg.ZAICodingRegion
	}
	return cfg.ZAIRegion
}

// SetZAIRegion persists the region for the given Z.AI gateway and syncs env + base URL for probe/discovery.
func SetZAIRegion(providerID, region string) error {
	normalized, err := zai.NormalizeRegion(region)
	if err != nil {
		return err
	}

	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		cfg = &eyriecfg.ProviderConfig{}
	}

	if providerID == ProviderZAICoding {
		cfg.ZAICodingRegion = string(normalized)
	} else {
		cfg.ZAIRegion = string(normalized)
	}

	if saveErr := eyriecfg.SaveProviderConfig(cfg, ""); saveErr != nil {
		return saveErr
	}

	// Set a region env for any code that wants it
	_ = os.Setenv("ZAI_REGION", string(normalized))

	// Derive and set the appropriate base override so fetchers and client pick the right host
	plan, _ := zai.PlanForProvider(providerID)
	base, err := zai.ResolveOpenAIBase(plan, normalized, "")
	if err == nil && base != "" {
		if providerID == ProviderZAICoding {
			_ = os.Setenv("ZAI_CODING_BASE_URL", base)
			cfg.ZAICodingBaseURL = base
		} else {
			_ = os.Setenv("ZAI_BASE_URL", base)
			cfg.ZAIBaseURL = base
		}
		_ = eyriecfg.SaveProviderConfig(cfg, "")
	}
	return nil
}

// ZAIRegionLabel returns the saved region for UI or "" .
func ZAIRegionLabel(providerID string) string {
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return ""
	}
	r := zaiRegionFromConfig(cfg, providerID)
	norm, err := zai.NormalizeRegion(r)
	if err != nil {
		return ""
	}
	return string(norm)
}

// ApplyZAIRegionEnv sets process envs from provider.json before credential probe / live fetch.
func ApplyZAIRegionEnv(ctx context.Context) {
	_ = ctx
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return
	}

	// General
	if r := strings.TrimSpace(cfg.ZAIRegion); r != "" {
		_ = os.Setenv("ZAI_REGION", r)
		if base, err := zai.ResolveOpenAIBase(zai.PlanGeneral, zai.Region(r), cfg.ZAIBaseURL); err == nil && base != "" {
			_ = os.Setenv("ZAI_BASE_URL", base)
		}
	}

	// Coding Plan (higher priority override if both present)
	if r := strings.TrimSpace(cfg.ZAICodingRegion); r != "" {
		_ = os.Setenv("ZAI_REGION", r)
		plan := zai.PlanCoding
		if base, err := zai.ResolveOpenAIBase(plan, zai.Region(r), cfg.ZAICodingBaseURL); err == nil && base != "" {
			_ = os.Setenv("ZAI_CODING_BASE_URL", base)
		}
	}
}
