package config

import (
	"context"
	"fmt"
	"sort"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/eyrie/setup"
)

// PersistAPIKey saves a provider API key via eyrie (OS secret store).
func PersistAPIKey(ctx context.Context, envKey, secret string) error {
	secret = strings.TrimSpace(secret)
	envKey = strings.TrimSpace(envKey)
	if secret == "" || envKey == "" {
		return nil
	}
	if err := eyriecfg.ValidateCredentialSecret(envKey, secret); err != nil {
		return err
	}
	return runtime.SetCredential(ctx, envKey, secret)
}

// PrepareCredentialDiscovery migrates any legacy ~/.hawk/env keys into the OS secret store.
func PrepareCredentialDiscovery(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, _ = credentials.MigrateLegacyEnvFile(ctx)
}

// ModelOption is one hawk /config model row.
type ModelOption struct {
	ID          string
	DisplayName string
}

// CredentialInference is one eyrie provider match for a pasted API key.
type CredentialInference struct {
	ProviderID   string
	DeploymentID string
	EnvVar       string
	DisplayName  string
}

// CredentialProviderOption is one eyrie provider row for /config pickers.
type CredentialProviderOption struct {
	ProviderID   string
	DeploymentID string
	EnvVar       string
	DisplayName  string
	Inferred     bool
	RequiresKey  bool
	Rank         int
}

// CredentialResolveResult is eyrie paste-key resolution (all providers + inferred hints).
type CredentialResolveResult struct {
	FormatOK    bool
	FormatError string
	Providers   []CredentialProviderOption
}

// ResolveCredential validates format and lists all providers from eyrie registry.
func ResolveCredential(ctx context.Context, secret string) CredentialResolveResult {
	res := runtime.ResolveCredential(ctx, secret)
	out := CredentialResolveResult{
		FormatOK:    res.FormatOK,
		FormatError: res.FormatError,
		Providers:   make([]CredentialProviderOption, len(res.Providers)),
	}
	for i, p := range res.Providers {
		out.Providers[i] = CredentialProviderOption{
			ProviderID:   p.ProviderID,
			DeploymentID: p.DeploymentID,
			EnvVar:       p.EnvVar,
			DisplayName:  p.DisplayName,
			Inferred:     p.Inferred,
			RequiresKey:  p.RequiresKey,
			Rank:         p.Rank,
		}
	}
	return out
}

// InferenceFromOption converts a provider picker row to persistence metadata.
func InferenceFromOption(opt CredentialProviderOption) CredentialInference {
	return CredentialInference{
		ProviderID:   opt.ProviderID,
		DeploymentID: opt.DeploymentID,
		EnvVar:       opt.EnvVar,
		DisplayName:  opt.DisplayName,
	}
}

// SaveCredential validates, probes, and stores via eyrie keychain.
func SaveCredential(ctx context.Context, inference CredentialInference, secret string) error {
	return runtime.SaveCredential(ctx, runtime.CredentialInference(inference), secret)
}

// ConfiguredCredentialProviders returns catalog providers with a stored API key.
func ConfiguredCredentialProviders() []string {
	var out []string
	for _, p := range AllCatalogProviders() {
		if EnvKeyStatus(p) == "set" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// FormatCredentialCLIStatus returns hawk credentials status output (providers, not raw env names).
func FormatCredentialCLIStatus(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	report := credentials.StorageReportFor(ctx)
	var b strings.Builder
	fmt.Fprintf(&b, "Credential storage: %s only\n", report.PlatformStore)
	if report.KeychainWritable {
		b.WriteString("  Keychain: writable\n")
	} else {
		fmt.Fprintf(&b, "  Keychain: %s\n", report.KeychainDetail)
	}
	providers := ConfiguredCredentialProviders()
	if len(providers) == 0 {
		b.WriteString("  Configured: (none)\n")
	} else {
		fmt.Fprintf(&b, "  Configured: %s\n", strings.Join(providers, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// RemoveStoredCredential deletes stored API key(s) for a provider name or env var.
func RemoveStoredCredential(ctx context.Context, target string) ([]string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("provider or env var name required")
	}
	envKeys := credentialEnvKeysForTarget(target)
	if len(envKeys) == 0 {
		return nil, fmt.Errorf("unknown provider %q", target)
	}
	var removed []string
	for _, envKey := range envKeys {
		if !credentials.HasSecret(ctx, envKey) {
			continue
		}
		if err := credentials.DeleteSecret(ctx, envKey); err != nil {
			return removed, err
		}
		removed = append(removed, envKey)
	}
	if len(removed) == 0 {
		return nil, fmt.Errorf("no stored credential for %q", target)
	}
	return removed, nil
}

func credentialEnvKeysForTarget(target string) []string {
	if strings.Contains(target, "_") && strings.ToUpper(target) == target {
		return []string{strings.TrimSpace(target)}
	}
	provider := catalogProviderID(normalizeProviderName(target))
	seen := map[string]struct{}{}
	var keys []string
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	if primary := ProviderAPIKeyEnv(provider); primary != "" {
		add(primary)
	}
	for _, alt := range providerCredentialEnvAliases(provider) {
		add(alt)
	}
	return keys
}

// LocalCredentialInference returns setup metadata for no-key providers (e.g. Ollama).
func LocalCredentialInference(providerID string) (CredentialInference, error) {
	inf, err := runtime.LocalCredentialInference(providerID)
	if err != nil {
		return CredentialInference{}, err
	}
	return CredentialInference{
		ProviderID:   inf.ProviderID,
		DeploymentID: inf.DeploymentID,
		EnvVar:       inf.EnvVar,
		DisplayName:  inf.DisplayName,
	}, nil
}

// FormatConfigProviderError maps eyrie setup errors to user-facing /config hints.
func FormatConfigProviderError(providerID string, err error) string {
	if err == nil {
		return ""
	}
	if formatted := runtime.FormatSetupError(providerID, err); formatted != nil {
		return formatted.Error()
	}
	return err.Error()
}

// InferCredentialsFromAPIKey delegates provider detection to eyrie from key shape + catalog.
func InferCredentialsFromAPIKey(ctx context.Context, secret string) []CredentialInference {
	in := runtime.InferCredentialsFromAPIKey(ctx, secret)
	out := make([]CredentialInference, len(in))
	for i, c := range in {
		out[i] = CredentialInference{
			ProviderID:   c.ProviderID,
			DeploymentID: c.DeploymentID,
			EnvVar:       c.EnvVar,
			DisplayName:  c.DisplayName,
		}
	}
	return out
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
