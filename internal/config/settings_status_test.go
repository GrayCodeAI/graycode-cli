package config

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestEnvKeyStatusUsesEyrieCredentialStatus(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	if got := EnvKeyStatus("openai"); got != "empty" {
		t.Fatalf("EnvKeyStatus without credential = %q, want empty", got)
	}
	if err := store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "sk-test-key"); err != nil {
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
