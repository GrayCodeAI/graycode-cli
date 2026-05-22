package eyrieclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

func TestBuildChatClientWithDeploymentRouting(t *testing.T) {
	dir := t.TempDir()
	writeProviderConfig(t, dir)
	t.Setenv("HOME", dir)
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("HAWK_DEPLOYMENT_ROUTING", "true")
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_MODEL_CATALOG_REFRESH", "")

	_, _, deploymentRouting := BuildChatClient(context.Background(), true, "openai")
	if !deploymentRouting {
		t.Fatal("expected deployment routing when useDeploymentRouting=true")
	}
}

func TestBuildChatClientWithoutDeploymentRouting(t *testing.T) {
	dir := t.TempDir()
	writeProviderConfig(t, dir)
	t.Setenv("HOME", dir)
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("HAWK_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	t.Setenv("EYRIE_MODEL_CATALOG_REFRESH", "")

	_, _, deploymentRouting := BuildChatClient(context.Background(), false, "openai")
	if deploymentRouting {
		t.Fatal("expected legacy client when useDeploymentRouting=false")
	}
}
