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
	marker := path + ".secrets-migrated"
	backup := path + ".pre-secret-migrate.bak"
	if _, err := os.Stat(marker); err == nil {
		// Migration already done: remove any leftover plaintext backup from
		// earlier versions that kept it around.
		_ = os.Remove(backup)
		return nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is eyrie's fixed provider config path, not external input
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
		_ = os.WriteFile(marker, []byte("ok\n"), 0o600)
		return nil
	}
	// Keep a backup only while the rewrite is in flight; it holds plaintext
	// secrets, so it must not outlive a successful migration.
	_ = os.WriteFile(backup, data, 0o600)
	if err := eyriecfg.SaveProviderConfig(&cfg, path); err != nil {
		return err
	}
	_ = os.WriteFile(marker, []byte("ok\n"), 0o600)
	_ = os.Remove(backup)
	return nil
}

func deploymentHasSecrets(dep eyriecfg.DeploymentConfig) bool {
	return strings.TrimSpace(dep.APIKey) != "" ||
		strings.TrimSpace(dep.Token) != "" ||
		strings.TrimSpace(dep.SecretAccessKey) != "" ||
		strings.TrimSpace(dep.AccessKeyID) != "" ||
		strings.TrimSpace(dep.SessionToken) != ""
}
