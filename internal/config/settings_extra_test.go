package config

import (
	"os"
	"testing"
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
