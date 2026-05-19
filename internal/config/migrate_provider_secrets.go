package config

import (
	"encoding/json"
	"os"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

// MigrateProviderSecrets strips api keys from on-disk provider.json (one-time hygiene).
func MigrateProviderSecrets() error {
	path := eyriecfg.GetProviderConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg eyriecfg.ProviderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	changed := false
	for id, dep := range cfg.Deployments {
		if deploymentHasSecrets(dep) {
			changed = true
		}
		cfg.Deployments[id] = eyriecfg.SanitizeDeploymentConfigForDisk(dep)
	}
	if !changed {
		return nil
	}
	backup := path + ".pre-secret-migrate.bak"
	_ = os.WriteFile(backup, data, 0o600)
	return eyriecfg.SaveProviderConfig(&cfg, path)
}

func deploymentHasSecrets(dep eyriecfg.DeploymentConfig) bool {
	return strings.TrimSpace(dep.APIKey) != "" ||
		strings.TrimSpace(dep.Token) != "" ||
		strings.TrimSpace(dep.SecretAccessKey) != "" ||
		strings.TrimSpace(dep.AccessKeyID) != "" ||
		strings.TrimSpace(dep.SessionToken) != ""
}

