package config

import (
	"context"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/eyrie/setup"
)

// PersistAPIKey saves a provider API key via eyrie (keychain + env fallback) and updates process env.
func PersistAPIKey(ctx context.Context, envKey, secret string) error {
	secret = strings.TrimSpace(secret)
	envKey = strings.TrimSpace(envKey)
	if secret == "" || envKey == "" {
		return nil
	}
	if err := eyriecfg.ValidateCredentialSecret(envKey, secret); err != nil {
		return err
	}
	if err := runtime.SetCredential(ctx, envKey, secret); err != nil {
		return err
	}
	if !SecureCredentialsEnabled() {
		return SaveEnvFile(envKey, secret)
	}
	return nil
}

// PrepareCredentialDiscovery loads keychain and ~/.hawk/env into the process before discover.
func PrepareCredentialDiscovery(ctx context.Context) {
	_ = LoadEnvFile()
	credentials.ApplyToProcess(ctx, credentials.DefaultStore())
}

// ModelOption is one hawk /config model row.
type ModelOption struct {
	ID          string
	DisplayName string
}

// OptionsFromSetupUI builds picker rows; providerFilter limits to one provider.
func OptionsFromSetupUI(ui *setup.SetupUI, providerFilter string) []ModelOption {
	if ui == nil {
		return nil
	}
	providerFilter = strings.TrimSpace(providerFilter)
	var out []ModelOption
	for _, p := range ui.Providers {
		if providerFilter != "" && p.ID != providerFilter {
			continue
		}
		for _, m := range p.Models {
			out = append(out, ModelOption{
				ID:          m.CanonicalID,
				DisplayName: m.DisplayName,
			})
		}
	}
	return out
}
