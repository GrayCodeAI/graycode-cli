package config

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

func TestEnvKeyStatusUsesGraycodeRouterCredentialStatus(t *testing.T) {
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	if got := EnvKeyStatus("openai"); got != "empty" {
		t.Fatalf("EnvKeyStatus without credential = %q, want empty", got)
	}
	if err := store.Set(context.Background(), gateway.AccountForEnv("OPENAI_API_KEY"), "sk-live-status-test-1234567890"); err != nil {
		t.Fatal(err)
	}
	if got := EnvKeyStatus("openai"); got != "set" {
		t.Fatalf("EnvKeyStatus with credential = %q, want set", got)
	}
}

func TestAllEnvKeyStatus(t *testing.T) {
	if result := AllEnvKeyStatus(); result == "" {
		t.Fatal("AllEnvKeyStatus should return a status string")
	}
}
