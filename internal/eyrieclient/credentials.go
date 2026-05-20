package eyrieclient

import (
	"context"
	"fmt"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/runtime"
)

// CredentialInference re-export.
type CredentialInference = runtime.CredentialInference

// CredentialResolveResult re-export.
type CredentialResolveResult = runtime.CredentialResolveResult

// CredentialProviderOption re-export.
type CredentialProviderOption = runtime.CredentialProviderOption

// InferenceFromOption converts a provider picker row to persistence metadata.
func InferenceFromOption(opt CredentialProviderOption) CredentialInference {
	return eyriecfg.InferenceFromOption(eyriecfg.CredentialProviderOption(opt))
}

// ResolveCredential validates format and lists providers.
func ResolveCredentialForHost(ctx context.Context, secret string) CredentialResolveResult {
	return runtime.ResolveCredential(ctx, secret)
}

// SaveCredentialForHost validates, probes, and stores a credential.
func SaveCredentialForHost(ctx context.Context, inference CredentialInference, secret string) error {
	return runtime.SaveCredential(ctx, inference, secret)
}

// FormatApplySummary returns a short status line after credential apply.
func FormatApplySummary(result *runtime.ApplyResult) string {
	if result == nil || result.Catalog == nil || result.Catalog.Compiled == nil {
		return "Eyrie credentials applied"
	}
	nModels := len(result.Catalog.Compiled.ModelsByID)
	nDeps := 0
	if result.Provider != nil {
		nDeps = len(result.Provider.Deployments)
	}
	return fmt.Sprintf("Eyrie: %d models, %d deployments configured, routing updated → %s",
		nModels, nDeps, result.ProviderPath)
}

// PrepareDiscovery ensures legacy plaintext credential files are migrated into the OS store.
func PrepareDiscovery(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, _ = credentials.MigrateLegacyEnvFile(ctx)
}
