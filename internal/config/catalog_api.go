package config

import (
	"context"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// CompiledCatalogV1 loads the eyrie catalog from cache or bootstrap wiring (no network).
func CompiledCatalogV1() *catalog.CompiledCatalogV1 {
	return compiledCatalogOrBootstrap()
}

func compiledCatalogOrBootstrap() *catalog.CompiledCatalogV1 {
	compiled, err := loadEyrieCatalogV1(context.Background(), false)
	if err == nil && compiled != nil {
		return compiled
	}
	bootstrap := catalog.BootstrapCatalogV1()
	compiled, err = catalog.CompileCatalogV1(&bootstrap)
	if err != nil {
		return nil
	}
	return compiled
}

// AllCatalogProviders returns provider IDs from eyrie (providers + deployments, not hawk constants).
func AllCatalogProviders() []string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, id := range catalog.ProviderIDsFromCompiled(compiled) {
		p := catalogProviderID(id)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// DefaultModelForProvider returns the first canonical model for a provider from eyrie's catalog.
func DefaultModelForProvider(provider string) string {
	ids, _ := ModelIDsForProvider(provider)
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// ModelIDsForProvider lists canonical model IDs for a provider from the eyrie JSON catalog.
func ModelIDsForProvider(provider string) ([]string, error) {
	entries, err := FetchModelsForProvider(provider)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ID != "" {
			out = append(out, e.ID)
		}
	}
	return out, nil
}

// CheapestModelForProvider picks the lowest input-priced model from eyrie's catalog.
func CheapestModelForProvider(provider, fallback string) string {
	entries, err := FetchModelsForProvider(provider)
	if err != nil || len(entries) == 0 {
		return fallback
	}
	cheapest := entries[0]
	for _, e := range entries[1:] {
		if e.InputPricePer1M > 0 && (cheapest.InputPricePer1M == 0 || e.InputPricePer1M < cheapest.InputPricePer1M) {
			cheapest = e
		}
	}
	if cheapest.ID != "" {
		return cheapest.ID
	}
	return fallback
}

// ProviderOfModel resolves catalog provider for a canonical model ID or alias.
func ProviderOfModel(modelName string) string {
	compiled := CompiledCatalogV1()
	if compiled == nil {
		return ""
	}
	if canonical, ok := compiled.CanonicalModelForAliasOrID(modelName); ok {
		if model := compiled.ModelsByID[canonical]; model.ID != "" {
			return catalogProviderID(model.ProviderID)
		}
	}
	return ""
}

// ExampleModelHints returns short example model aliases for user-facing error messages.
func ExampleModelHints() (anthropic, openai string) {
	compiled := CompiledCatalogV1()
	if compiled == nil {
		return "claude-sonnet-4-6", "gpt-4o"
	}
	if _, ok := compiled.CanonicalModelForAliasOrID("claude-sonnet-4-6"); ok {
		anthropic = "claude-sonnet-4-6"
	}
	if _, ok := compiled.CanonicalModelForAliasOrID("gpt-4o"); ok {
		openai = "gpt-4o"
	}
	if anthropic == "" || openai == "" {
		for _, id := range []string{"anthropic/claude-sonnet-4-6", "openai/gpt-4o"} {
			if _, ok := compiled.ModelsByID[id]; !ok {
				continue
			}
			if strings.HasPrefix(id, "anthropic/") && anthropic == "" {
				anthropic = strings.TrimPrefix(id, "anthropic/")
			}
			if strings.HasPrefix(id, "openai/") && openai == "" {
				openai = strings.TrimPrefix(id, "openai/")
			}
		}
	}
	if anthropic == "" || openai == "" {
		return "claude-sonnet-4-6", "gpt-4o"
	}
	return anthropic, openai
}

// AllCanonicalModelIDs returns sorted canonical model IDs from the eyrie catalog.
func AllCanonicalModelIDs() []string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return nil
	}
	out := make([]string, 0, len(compiled.ModelsByID))
	for id := range compiled.ModelsByID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ProviderIDForDeployment returns the catalog provider id for a deployment (e.g. anthropic-direct → anthropic).
func ProviderIDForDeployment(deploymentID string) string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return ""
	}
	dep, ok := compiled.DeploymentsByID[deploymentID]
	if !ok {
		return ""
	}
	return catalogProviderID(dep.ProviderID)
}

// PrimaryAPIKeyEnvForDeployment returns the env var name for a deployment's API key.
func PrimaryAPIKeyEnvForDeployment(deploymentID string) string {
	compiled := compiledCatalogOrBootstrap()
	if compiled == nil {
		return ""
	}
	return catalog.PrimaryAPIKeyEnvForDeployment(compiled, deploymentID)
}

// ConfigProviderList returns provider names for the /config UI from catalog + custom providers.
func ConfigProviderList(custom []CustomProviderConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range AllCatalogProviders() {
		engine := NormalizeProviderForEngine(p)
		if engine == "" || seen[engine] {
			continue
		}
		seen[engine] = true
		out = append(out, engine)
	}
	for _, cp := range custom {
		name := strings.TrimSpace(cp.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
