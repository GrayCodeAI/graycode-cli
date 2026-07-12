package config

import (
	"context"
	"fmt"
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
	if err := runtime.SetCredential(ctx, envKey, secret); err != nil {
		return err
	}
	InvalidateConfigUICache()
	return nil
}

// PrepareCredentialDiscovery prepares runtime credential discovery without reading legacy files.
func PrepareCredentialDiscovery(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.PrepareCredentialDiscovery(ctx)
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

// CredentialResolveResult is eyrie paste-key resolution (format check + full provider list; no prefix inference).
type CredentialResolveResult struct {
	FormatOK                bool
	FormatError             string
	Providers               []CredentialProviderOption
	ProbeDisambiguationUsed bool
}

// ResolveCredential validates format and lists all providers from eyrie registry.
func ResolveCredential(ctx context.Context, secret string) CredentialResolveResult {
	res := runtime.ResolveCredential(ctx, secret)
	out := CredentialResolveResult{
		FormatOK:                res.FormatOK,
		FormatError:             res.FormatError,
		ProbeDisambiguationUsed: res.ProbeDisambiguationUsed,
		Providers:               make([]CredentialProviderOption, len(res.Providers)),
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
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	if _, err := engine.SaveCredential(ctx, inference.ProviderID, secret); err != nil {
		return err
	}
	InvalidateConfigUICache()
	return nil
}

// HasStoredCredentialForProvider reports whether the OS secret store has a key for this gateway.
func HasStoredCredentialForProvider(ctx context.Context, providerID string) bool {
	return runtime.HasStoredCredential(ctx, providerID)
}

// ConfiguredCredentialProviders returns setup gateways with a stored API key.
func ConfiguredCredentialProviders() []string {
	return configuredCredentialProvidersCached(context.Background())
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
			if len(removed) > 0 {
				InvalidateConfigUICache()
			}
			return removed, err
		}
		removed = append(removed, envKey)
	}
	if len(removed) == 0 {
		return nil, fmt.Errorf("no stored credential for %q", target)
	}
	InvalidateConfigUICache()
	return removed, nil
}

// MaskCredentialForProvider returns a partially masked API key for UI display.
func MaskCredentialForProvider(ctx context.Context, provider string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, envKey := range credentialEnvKeysForTarget(provider) {
		secret := credentials.LookupSecret(ctx, envKey)
		if secret == "" {
			continue
		}
		return maskCredentialSecret(secret)
	}
	return "••••••••"
}

func maskCredentialSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "••••••••"
	}
	if len(secret) <= 8 {
		return strings.Repeat("•", len(secret))
	}
	// Show only the last 4 characters; a fixed bullet count hides both the
	// provider-identifying prefix and the secret's true length.
	return strings.Repeat("•", 8) + secret[len(secret)-4:]
}

// CredentialInferenceForProvider returns save metadata for a gateway chosen in /config.
func CredentialInferenceForProvider(providerID string) (CredentialInference, error) {
	providerID = runtime.SetupGatewayID(providerID)
	inf, err := runtime.InferenceForProvider(providerID)
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

func credentialEnvKeysForTarget(target string) []string {
	if strings.Contains(target, "_") && strings.ToUpper(target) == target {
		return []string{strings.TrimSpace(target)}
	}
	return runtime.CredentialEnvKeys(target)
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

// InferCredentialsFromAPIKey is deprecated; select gateway first, then paste the key.
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
