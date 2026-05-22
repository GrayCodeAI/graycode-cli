// Package eyrieclient is hawk's only integration with eyrie.
// Hawk must not import eyrie/catalog, eyrie/setup, or eyrie/config directly — use runtime here.
package eyrieclient

import (
	"context"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/eyrie/setup"
)

// LoadRuntime reads eyrie catalog + provider.json from disk (no network).
func LoadRuntime(ctx context.Context) (*runtime.Runtime, error) {
	return runtime.Load(ctx)
}

// Discover refreshes the catalog from API keys and rewrites provider routing.
func Discover(ctx context.Context) (*runtime.ApplyResult, error) {
	return runtime.Discover(ctx)
}

// ApplyCredentials is the same as Discover (paste-key / refresh flows).
func ApplyCredentials(ctx context.Context) (*runtime.ApplyResult, error) {
	return runtime.Apply(ctx, eyriecfg.DiscoveryCredentials(ctx))
}

// SetAPIKey stores a secret in eyrie keychain (validated by eyrie).
func SetAPIKey(ctx context.Context, envKey, secret string) error {
	return runtime.SetCredential(ctx, envKey, secret)
}

// ListCatalogModels returns cached catalog models (legacy; prefer ListModelsForProvider).
func ListCatalogModels(ctx context.Context, provider string) ([]catalog.ModelCatalogEntry, error) {
	return runtime.ModelsForProvider(ctx, provider)
}

// ListDeployments returns deployment rows with credential status.
func ListDeployments(ctx context.Context) ([]runtime.DeploymentRow, error) {
	rt, err := runtime.Load(ctx)
	if err != nil {
		return nil, err
	}
	return rt.DeploymentRows()
}

// SetupUI returns provider/model groups for /config pickers.
func SetupUI(ctx context.Context, providerFilter string) (*setup.SetupUI, error) {
	return runtime.SetupUIFromCatalog(ctx, providerFilter)
}

// PrimaryAPIKeyEnvForDeployment resolves env var name from eyrie catalog.
func PrimaryAPIKeyEnvForDeployment(deploymentID string) string {
	return runtime.PrimaryAPIKeyEnv(deploymentID)
}

// ProviderIDForDeployment resolves provider id for a deployment.
func ProviderIDForDeployment(deploymentID string) string {
	return runtime.ProviderIDForDeployment(deploymentID)
}

// DefaultModelProviderFilter returns the provider id to use when listing models with no filter.
func DefaultModelProviderFilter(ctx context.Context) string {
	return runtime.DefaultModelProviderFilter(ctx)
}

// InferCredentialsFromAPIKey returns prefix-inferred provider candidates.
func InferCredentialsFromAPIKey(ctx context.Context, secret string) []runtime.CredentialInference {
	return runtime.InferCredentialsFromAPIKey(ctx, secret)
}

// ResolveCredential lists all providers with inferred hints (paste-key setup).
func ResolveCredential(ctx context.Context, secret string) runtime.CredentialResolveResult {
	return runtime.ResolveCredential(ctx, secret)
}

// SaveCredential validates, probes, and stores a key in eyrie keychain.
func SaveCredential(ctx context.Context, inference runtime.CredentialInference, secret string) error {
	return runtime.SaveCredential(ctx, inference, secret)
}

// ClearActiveSelection resets the active model/provider selection.
func ClearActiveSelection(ctx context.Context) error {
	return runtime.ClearActiveSelection(ctx)
}

// HasAnyConfiguredDeployment reports whether at least one deployment has credentials.
func HasAnyConfiguredDeployment(ctx context.Context) bool {
	return eyriecfg.HasAnyConfiguredDeployment(ctx)
}
