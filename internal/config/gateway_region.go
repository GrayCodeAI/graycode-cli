package config

import (
	"context"
)

// NeedsGatewayRegion reports whether a gateway still needs a region selection.
func NeedsGatewayRegion(providerID string) bool {
	_, required := EngineGatewayRegion(context.Background(), providerID)
	return required
}

// SetGatewayRegion persists the selected region for a regional gateway.
func SetGatewayRegion(providerID, region string) error {
	return SetEngineGatewayRegion(context.Background(), providerID, region)
}

// GatewayRegionLabel returns the saved region label for a gateway or "".
func GatewayRegionLabel(providerID string) string {
	label, _ := EngineGatewayRegion(context.Background(), providerID)
	return label
}
