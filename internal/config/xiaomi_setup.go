package config

import (
	"context"
)

const ProviderXiaomiTokenPlan = "xiaomi_mimo_token_plan" // #nosec G101 -- provider ID string, not a credential

// NeedsXiaomiTokenPlanRegion reports whether the Token Plan gateway still needs a cluster pick.
func NeedsXiaomiTokenPlanRegion(providerID string) bool {
	_, required := EngineGatewayRegion(providerID)
	return required
}

// SetXiaomiTokenPlanRegion persists region (cn, sgp, ams). Eyrie reads the
// provider state directly when probing; Hawk does not mutate process env.
func SetXiaomiTokenPlanRegion(region string) error {
	return SetEngineGatewayRegion(context.Background(), ProviderXiaomiTokenPlan, region)
}

// XiaomiTokenPlanRegionLabel returns the saved cluster id for UI (cn, sgp, ams) or "" if unset.
func XiaomiTokenPlanRegionLabel() string {
	label, _ := EngineGatewayRegion(ProviderXiaomiTokenPlan)
	return label
}
