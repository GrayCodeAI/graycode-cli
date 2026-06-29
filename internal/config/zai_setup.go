package config

import (
	"context"
	"github.com/GrayCodeAI/eyrie/runtime"
)

const (
	ProviderZAIPayg   = "zai_payg"
	ProviderZAICoding = "zai_coding"
)

// NeedsZAIRegion reports whether the Z.AI gateway still needs a region pick for the chosen plan.
func NeedsZAIRegion(providerID string) bool {
	return runtime.GatewayNeedsRegion(providerID)
}

// SetZAIRegion persists the region (international or cn) for the given Z.AI gateway and syncs env + derived base.
func SetZAIRegion(providerID, region string) error {
	return runtime.SetGatewayRegion(providerID, region)
}

// ZAIRegionLabel returns the saved region label or "".
func ZAIRegionLabel(providerID string) string {
	return runtime.GatewayRegionLabel(providerID)
}

// ApplyZAIRegionEnv sets process envs from provider.json before probe/fetch/chat.
func ApplyZAIRegionEnv(ctx context.Context) {
	runtime.ApplyGatewayEnv(ctx, ProviderZAIPayg)
	runtime.ApplyGatewayEnv(ctx, ProviderZAICoding)
}
