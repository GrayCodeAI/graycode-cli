package config

import (
	"os"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
)

func TestNormalizeProviderName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"anthropic", "anthropic"},
		{"Anthropic", "anthropic"},
		{"OPENAI", "openai"},
		{"openai", "openai"},
		{"gemini", "gemini"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeProviderName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeProviderName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBoolPtr(t *testing.T) {
	t.Parallel()
	p := BoolPtr(true)
	if p == nil || !*p {
		t.Error("BoolPtr(true) should return pointer to true")
	}
	p2 := BoolPtr(false)
	if p2 == nil || *p2 {
		t.Error("BoolPtr(false) should return pointer to false")
	}
}

func TestProviderAPIKeyEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		provider string
		want     string
	}{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"gemini", "GEMINI_API_KEY"},
	}
	for _, tt := range tests {
		got := ProviderAPIKeyEnv(tt.provider)
		if got != tt.want {
			t.Errorf("ProviderAPIKeyEnv(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestNormalizeProviderForEngine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"google", "google"},
		{"gemini", "gemini"},
	}
	for _, tt := range tests {
		got := NormalizeProviderForEngine(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeProviderForEngine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFetchModelsForProviderUsesEyrieJSONCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Now().UTC().Truncate(time.Second)
	c := catalog.CatalogV1{
		SchemaVersion: catalog.CatalogV1SchemaVersion,
		GeneratedAt:   now,
		StaleAfter:    now.Add(time.Hour),
		Providers: map[string]catalog.ProviderV1{
			"openai": {ID: "openai", Name: "OpenAI"},
		},
		APIProtocols: map[string]catalog.APIProtocolV1{
			"openai-chat-completions": {ID: "openai-chat-completions", Name: "OpenAI Chat Completions"},
		},
		Deployments: map[string]catalog.DeploymentV1{
			"openai-direct": {
				ID:                    "openai-direct",
				Name:                  "OpenAI",
				ProviderID:            "openai",
				APIProtocolID:         "openai-chat-completions",
				AdapterConstructor:    "openai",
				NativeModelIDSource:   catalog.NativeModelIDCatalogKnown,
				ModelMappingsRequired: false,
			},
		},
		Models: map[string]catalog.ModelV1{
			"openai/test-json-model": {
				ID:            "openai/test-json-model",
				ProviderID:    "openai",
				Name:          "Test JSON Model",
				ContextWindow: 12345,
				MaxOutput:     678,
			},
		},
		Offerings: []catalog.ModelOfferingV1{{
			ID:               "openai-direct:test-json-model",
			CanonicalModelID: "openai/test-json-model",
			DeploymentID:     "openai-direct",
			NativeModelID:    "test-json-model",
			Pricing: catalog.PricingV1{
				Status:     catalog.PricingKnown,
				Currency:   "USD",
				RatesPer1M: map[string]float64{"input_tokens": 1.25, "output_tokens": 2.5},
			},
		}},
	}
	if err := catalog.WriteCatalogV1Cache(eyrieModelCatalogCachePath(), &c); err != nil {
		t.Fatalf("write catalog cache: %v", err)
	}

	models, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("FetchModelsForProvider: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models len = %d, want 1", len(models))
	}
	if models[0].ID != "openai/test-json-model" {
		t.Fatalf("model ID = %q, want JSON cache model", models[0].ID)
	}
	if models[0].InputPricePer1M != 1.25 || models[0].OutputPricePer1M != 2.5 {
		t.Fatalf("pricing not read from JSON cache: %#v", models[0])
	}
}

func TestEnvKeyStatus(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	status := EnvKeyStatus("anthropic")
	if status == "" {
		t.Error("EnvKeyStatus should return non-empty")
	}
}

func TestAllEnvKeyStatus(t *testing.T) {
	result := AllEnvKeyStatus()
	if result == "" {
		t.Error("AllEnvKeyStatus should return status string")
	}
}

func TestAPIKeyForProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-key")
	key := APIKeyForProvider("openai")
	if key != "sk-test-key" {
		t.Errorf("APIKeyForProvider = %q, want sk-test-key", key)
	}
}

func TestAPIKeyForProvider_Missing(t *testing.T) {
	t.Setenv("NONEXISTENT_PROVIDER_API_KEY", "")
	os.Unsetenv("NONEXISTENT_PROVIDER_API_KEY")
	key := APIKeyForProvider("nonexistent_provider_xyz")
	if key != "" {
		t.Errorf("expected empty for missing key, got %q", key)
	}
}

func TestEnvFilePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := envFilePath()
	if path == "" {
		t.Error("envFilePath should return non-empty")
	}
}
