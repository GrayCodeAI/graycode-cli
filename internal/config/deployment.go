package config

import (
	"context"

	"github.com/GrayCodeAI/eyrie/runtime"
)

// DeploymentRoutingEnabled delegates deployment-routing policy ownership to Eyrie runtime.
func DeploymentRoutingEnabled(s Settings) bool {
	return runtime.DeploymentRoutingEnabled(context.Background(), s.DeploymentRouting)
}
