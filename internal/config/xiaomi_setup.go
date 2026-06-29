package config

import (
	"context"
	"github.com/GrayCodeAI/eyrie/runtime"
)

const ProviderXiaomiTokenPlan = "xiaomi_mimo_token_plan"

// NeedsXiaomiTokenPlanRegion reports whether the Token Plan gateway still needs a cluster pick.
func NeedsXiaomiTokenPlanRegion(providerID string) bool {
	return runtime.GatewayNeedsRegion(providerID)
}

// SetXiaomiTokenPlanRegion persists region (cn, sgp, ams) and syncs env for probe/discovery.
func SetXiaomiTokenPlanRegion(region string) error {
	return runtime.SetGatewayRegion(ProviderXiaomiTokenPlan, region)
}

// XiaomiTokenPlanRegionLabel returns the saved cluster id for UI (cn, sgp, ams) or "" if unset.
func XiaomiTokenPlanRegionLabel() string {
	return runtime.GatewayRegionLabel(ProviderXiaomiTokenPlan)
}

// ApplyXiaomiTokenPlanRegionEnv sets process env from provider.json before credential probe.
func ApplyXiaomiTokenPlanRegionEnv(ctx context.Context) {
	runtime.ApplyGatewayEnv(ctx, ProviderXiaomiTokenPlan)
}
