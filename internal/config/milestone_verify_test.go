package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

// isolateMilestoneTest uses a temp HOME and HAWK_CONFIG_DIR so verification does not touch the user machine.
func isolateMilestoneTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	hawkDir := filepath.Join(home, ".hawk")
	if err := os.MkdirAll(hawkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("HAWK_CONFIG_DIR", hawkDir)
	return hawkDir
}

func TestVerify_ProviderJSONOnDiskHasNoSecrets(t *testing.T) {
	isolateMilestoneTest(t)
	compiled := CompiledCatalogV1()
	if compiled == nil {
		t.Fatal("compiled catalog required")
	}
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-verify-test-key-1234567890"}
	cfg := eyriecfg.SyncProviderConfigFromCatalog(compiled, env)
	path := eyriecfg.GetProviderConfigPath()
	if err := eyriecfg.SaveProviderConfig(cfg, path); err != nil {
		t.Fatal(err)
	}
	assertProviderJSONFileHasNoSecrets(t, path)
}

func TestVerify_MigrateProviderSecretsStripsDisk(t *testing.T) {
	hawkDir := isolateMilestoneTest(t)
	path := filepath.Join(hawkDir, "provider.json")
	secret := "sk-ant-migrate-verify-key-1234567890"
	raw := `{
  "version": "1",
  "config_version": 2,
  "deployments": {
    "anthropic-direct": {
      "api_key": "` + secret + `"
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateProviderSecrets(); err != nil {
		t.Fatal(err)
	}
	assertProviderJSONFileHasNoSecrets(t, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("provider.json still contains api key after migrate")
	}
}

func TestVerify_PersistAPIKeyDoesNotWriteProviderJSON(t *testing.T) {
	hawkDir := isolateMilestoneTest(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	secret := "sk-ant-persist-verify-key-1234567890"
	if err := PersistAPIKey(context.Background(), "ANTHROPIC_API_KEY", secret); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hawkDir, "provider.json")
	if _, err := os.Stat(path); err == nil {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), secret) {
			t.Fatal("PersistAPIKey must not write secrets to provider.json")
		}
	}
}

func TestVerify_EvaluateSetupFlow(t *testing.T) {
	isolateMilestoneTest(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	compiled := CompiledCatalogV1()
	if compiled != nil {
		for _, k := range catalog.DiscoveryEnvKeysFromCatalog(compiled) {
			t.Setenv(k, "")
		}
	}

	st := EvaluateSetup(ctx)
	if !st.NeedsSetup || st.HasCredentials {
		t.Fatalf("expected setup needed without credentials, got %+v", st)
	}

	secret := "sk-ant-flow-verify-key-1234567890"
	if err := store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), secret); err != nil {
		t.Fatal(err)
	}
	st = EvaluateSetup(ctx)
	if !st.HasCredentials {
		t.Fatal("expected credentials after keychain key set")
	}
	if !st.NeedsSetup || st.HasModel {
		t.Fatal("expected setup still needed until model selected")
	}

	providerPath := filepath.Join(os.Getenv("HOME"), ".hawk", "provider.json")
	cfg := &eyriecfg.ProviderConfig{
		ActiveProvider: "anthropic",
		ActiveModel:    "claude-sonnet-4-20250514",
		AnthropicModel: "claude-sonnet-4-20250514",
	}
	if err := eyriecfg.SaveProviderConfig(cfg, providerPath); err != nil {
		t.Fatal(err)
	}
	st = EvaluateSetup(ctx)
	if st.NeedsSetup {
		t.Fatalf("expected setup complete with key + model, got %+v", st)
	}
}

func assertProviderJSONFileHasNoSecrets(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, needle := range []string{`"api_key"`, `"secret_access_key"`, `"session_token"`} {
		if !strings.Contains(text, needle) {
			continue
		}
		// Empty values are OK: "api_key": ""
		if strings.Contains(text, needle+`": ""`) || strings.Contains(text, needle+`":""`) {
			continue
		}
		if strings.Contains(text, needle+`": "`) && !strings.Contains(text, needle+`": ""`) {
			t.Fatalf("provider.json at %s contains non-empty %s", path, needle)
		}
	}
	var cfg eyriecfg.ProviderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for id, dep := range cfg.Deployments {
		if deploymentHasSecrets(dep) {
			t.Fatalf("deployment %q still has secret fields in struct", id)
		}
	}
}
