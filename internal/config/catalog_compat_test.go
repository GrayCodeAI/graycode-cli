package config

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyrieconfig "github.com/GrayCodeAI/eyrie/config"
)

// CompiledCatalogV1 is retained only for lower-level migration/security tests.
// Production Graycode code consumes engine DTOs exclusively.
func CompiledCatalogV1() *catalog.CompiledCatalog {
	compiled, err := catalog.LoadCatalog(context.Background(), catalog.LoadCatalogOptions{CachePath: catalog.DefaultCachePath()})
	if err == nil && compiled != nil {
		return compiled
	}
	bootstrap := catalog.BootstrapCatalog()
	compiled, _ = catalog.CompileCatalog(&bootstrap)
	return compiled
}

func deploymentHasSecrets(deployment eyrieconfig.DeploymentConfig) bool {
	return strings.TrimSpace(deployment.APIKey) != "" || strings.TrimSpace(deployment.Token) != "" ||
		strings.TrimSpace(deployment.SecretAccessKey) != "" || strings.TrimSpace(deployment.AccessKeyID) != "" ||
		strings.TrimSpace(deployment.SessionToken) != ""
}
