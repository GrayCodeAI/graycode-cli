package eyrieclient

import (
	"context"
	"encoding/json"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
)

type (
	ModelCatalogEntry = catalog.ModelCatalogEntry
	CompiledCatalogV1 = catalog.CompiledCatalogV1
	CatalogV1         = catalog.CatalogV1
)

func DisplayModelLabel(id, displayName string) string {
	return catalog.DisplayModelLabel(id, displayName)
}

func DisplayModelOwner(owner string, id string, liveMetadata ...json.RawMessage) string {
	if len(liveMetadata) > 0 {
		return catalog.DisplayModelOwner(owner, id, liveMetadata[0])
	}
	return catalog.DisplayModelOwner(owner, id)
}

func ModelEntriesForProvider(compiled *CompiledCatalogV1, provider string) []ModelCatalogEntry {
	return catalog.ModelEntriesForProvider(compiled, provider)
}

func PrimaryAPIKeyEnvForProvider(compiled *CompiledCatalogV1, provider string) string {
	return catalog.PrimaryAPIKeyEnvForProvider(compiled, provider)
}

func APIKeyEnvsForProvider(compiled *CompiledCatalogV1, provider string) []string {
	return catalog.APIKeyEnvsForProvider(compiled, provider)
}

func GatewayForModel(compiled *CompiledCatalogV1, modelID string) string {
	return catalog.GatewayForModel(compiled, modelID)
}

func ProviderIDsFromCompiled(compiled *CompiledCatalogV1) []string {
	return catalog.ProviderIDsFromCompiled(compiled)
}

func FirstModelForProvider(compiled *CompiledCatalogV1, provider string) string {
	return catalog.FirstModelForProvider(compiled, provider)
}

func IsLiveOnlyProvider(provider string) bool {
	return catalog.IsLiveOnlyProvider(provider)
}

func GetProviderDefaultModel(provider string) string {
	return catalog.GetProviderDefaultModel(provider, nil)
}

func FetchLiveModelEntriesForProvider(env map[string]string, provider string) ([]ModelCatalogEntry, error) {
	return catalog.FetchLiveModelEntriesForProvider(env, provider)
}

func LoadCompiledCatalogV1(ctx context.Context, opts catalog.LoadCatalogV1Options) (*CompiledCatalogV1, error) {
	return catalog.LoadCatalogV1(ctx, opts)
}

func DefaultCachePath() string {
	return catalog.DefaultCachePath()
}

func CacheInfo(cachePath string) (exists bool, modTime time.Time, size int64, err error) {
	return catalog.CacheInfo(cachePath)
}
