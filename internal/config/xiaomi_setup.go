package config

const ProviderXiaomiTokenPlan = "xiaomi_mimo_token_plan" // #nosec G101 -- provider ID string, not a credential

// NeedsXiaomiTokenPlanRegion reports whether the Token Plan gateway still needs a cluster pick.
func NeedsXiaomiTokenPlanRegion(providerID string) bool {
	return NeedsGatewayRegion(providerID)
}

// SetXiaomiTokenPlanRegion persists region (cn, sgp, ams). Eyrie reads the
// provider state directly when probing; Hawk does not mutate process env.
func SetXiaomiTokenPlanRegion(region string) error {
	return SetGatewayRegion(ProviderXiaomiTokenPlan, region)
}

// XiaomiTokenPlanRegionLabel returns the saved cluster id for UI (cn, sgp, ams) or "" if unset.
func XiaomiTokenPlanRegionLabel() string {
	return GatewayRegionLabel(ProviderXiaomiTokenPlan)
}
