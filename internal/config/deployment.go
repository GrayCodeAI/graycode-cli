package config

import (
	"os"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// DeploymentRoutingEnabled decides whether hawk uses catalog-backed deployment routing
// (same rules as eyrie CLI). HAWK_DEPLOYMENT_ROUTING overrides; otherwise settings flag,
// otherwise provider.json shape via eyrie/setup.
func DeploymentRoutingEnabled(s Settings) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("HAWK_DEPLOYMENT_ROUTING"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	if s.DeploymentRouting != nil {
		return *s.DeploymentRouting
	}
	cfg := eyriecfg.LoadProviderConfig("")
	return setup.UseDeploymentRouting(cfg)
}
