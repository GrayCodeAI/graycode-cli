package eyrieclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	hawkcfg "github.com/GrayCodeAI/hawk/internal/config"
)

func writeProviderConfig(t *testing.T, dir string) {
	t.Helper()
	data := []byte(`{
  "active_provider": "openai",
  "openai_api_key": "sk-test-key-for-routing"
}`)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), data, 0o600); err != nil {
		t.Fatalf("write provider config: %v", err)
	}
}

func TestBuildChatClientForcedDeploymentRoutingFromHawkEnv(t *testing.T) {
	dir := t.TempDir()
	writeProviderConfig(t, dir)
	t.Setenv("HOME", dir)
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("HAWK_DEPLOYMENT_ROUTING", "true")
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_MODEL_CATALOG_REFRESH", "")

	_, _, deploymentRouting := BuildChatClient(context.Background(), hawkcfg.Settings{}, "openai")
	if !deploymentRouting {
		t.Fatal("expected HAWK_DEPLOYMENT_ROUTING=true to force deployment routing")
	}
}

func TestBuildChatClientForcedDeploymentRoutingFromHawkSettings(t *testing.T) {
	dir := t.TempDir()
	writeProviderConfig(t, dir)
	t.Setenv("HOME", dir)
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("HAWK_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_MODEL_CATALOG_REFRESH", "")
	enabled := true

	_, _, deploymentRouting := BuildChatClient(context.Background(), hawkcfg.Settings{DeploymentRouting: &enabled}, "openai")
	if !deploymentRouting {
		t.Fatal("expected deployment_routing setting to force deployment routing")
	}
}

func TestBuildChatClientLegacyProviderConfigDefaultsToLegacyClient(t *testing.T) {
	dir := t.TempDir()
	writeProviderConfig(t, dir)
	t.Setenv("HOME", dir)
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("HAWK_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_MODEL_CATALOG_REFRESH", "")

	_, _, deploymentRouting := BuildChatClient(context.Background(), hawkcfg.Settings{}, "openai")
	if deploymentRouting {
		t.Fatal("legacy provider config should not enable deployment routing unless explicitly requested")
	}
}
