package config

import (
	"context"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/zai"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

const (
	ProviderZAIPayg   = "zai_payg"
	ProviderZAICoding = "zai_coding"
)

// NeedsZAIRegion reports whether the Z.AI gateway still needs a region pick for the chosen plan.
func NeedsZAIRegion(providerID string) bool {
	p := strings.TrimSpace(providerID)
	if p != ProviderZAICoding {
		return false
	}
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return true
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

// SetZAIRegion persists the region (international or cn) for the given Z.AI gateway and syncs env + derived base.
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

	_ = os.Setenv("ZAI_REGION", string(normalized))

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

// ZAIRegionLabel returns the saved region label or "".
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

// ApplyZAIRegionEnv sets process envs from provider.json before probe/fetch/chat.
func ApplyZAIRegionEnv(ctx context.Context) {
	_ = ctx
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return
	}

	// General
	if r := strings.TrimSpace(cfg.ZAIRegion); r != "" {
		_ = os.Setenv("ZAI_REGION", r)
		plan := zai.PlanGeneral
		norm, _ := zai.NormalizeRegion(r)
		if base, err := zai.ResolveOpenAIBase(plan, norm, cfg.ZAIBaseURL); err == nil && base != "" {
			_ = os.Setenv("ZAI_BASE_URL", base)
		}
	}

	// Coding Plan
	if r := strings.TrimSpace(cfg.ZAICodingRegion); r != "" {
		_ = os.Setenv("ZAI_CODING_REGION", r)
		plan := zai.PlanCoding
		norm, _ := zai.NormalizeRegion(r)
		if base, err := zai.ResolveOpenAIBase(plan, norm, cfg.ZAICodingBaseURL); err == nil && base != "" {
			_ = os.Setenv("ZAI_CODING_BASE_URL", base)
		}
	}
}
