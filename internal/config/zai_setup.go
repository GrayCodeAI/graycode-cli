package config

const (
	ProviderZAIPayg   = "zai_payg"
	ProviderZAICoding = "zai_coding"
)

// NeedsZAIRegion reports whether the Z.AI gateway still needs a region pick for the chosen plan.
func NeedsZAIRegion(providerID string) bool {
	return NeedsGatewayRegion(providerID)
}

// SetZAIRegion persists the region (international or cn) for the given Z.AI
// gateway. Eyrie reads provider state directly without process-env mutation.
func SetZAIRegion(providerID, region string) error {
	return SetGatewayRegion(providerID, region)
}

// ZAIRegionLabel returns the saved region label or "".
func ZAIRegionLabel(providerID string) string {
	return GatewayRegionLabel(providerID)
}
