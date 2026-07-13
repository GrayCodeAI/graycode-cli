package config

// MigrateProviderSecrets strips api keys from on-disk provider.json (one-time hygiene).
func MigrateProviderSecrets() error {
	return MigrateEngineProviderSecrets()
}
