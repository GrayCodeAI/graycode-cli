package config

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

func TestRemoveStoredCredential_ByProvider(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")

	removed, err := RemoveStoredCredential(ctx, "openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "OPENROUTER_API_KEY" {
		t.Fatalf("removed = %v", removed)
	}
	if gateway.HasSecret(ctx, "OPENROUTER_API_KEY") {
		t.Fatal("key should be deleted")
	}
}

func TestRemoveStoredCredential_ByEnvVar(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-1234567890")

	removed, err := RemoveStoredCredential(ctx, "ANTHROPIC_API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Fatalf("removed = %v", removed)
	}
}

func TestRemoveStoredCredential_NotFound(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	_, err := RemoveStoredCredential(context.Background(), "openrouter")
	if err == nil || !strings.Contains(err.Error(), "no stored credential") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestFormatCredentialCLIStatus(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	ctx := context.Background()
	_ = store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")

	out := FormatCredentialCLIStatus(ctx)
	if !strings.Contains(out, "Configured:") {
		t.Fatalf("expected configured section, got:\n%s", out)
	}
	if strings.Contains(out, "Keys stored:") {
		t.Fatal("should not show legacy key count")
	}
}

func TestMaskCredentialSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   string
	}{
		{"empty", "", "••••••••"},
		{"whitespace only", "   ", "••••••••"},
		{"short all bullets", "abcd1234", "••••••••"},
		{"long shows only last 4", "sk-ant-api03-secretvalue", "••••••••alue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskCredentialSecret(tt.secret); got != tt.want {
				t.Errorf("maskCredentialSecret(%q) = %q, want %q", tt.secret, got, tt.want)
			}
		})
	}
	long := maskCredentialSecret("sk-proj-1234567890abcdef")
	if strings.Contains(long, "sk-p") {
		t.Errorf("mask leaks key prefix: %q", long)
	}
}
