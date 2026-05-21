package config

import (
	"os"
	"path/filepath"
	"testing"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

func TestSaveRoutingPolicyJSONValidatesDeployments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	t.Setenv("HAWK_CONFIG_DIR", dir)

	cfg := &eyriecfg.ProviderConfig{
		ConfigVersion: 2,
		Deployments: map[string]eyriecfg.DeploymentConfig{
			"anthropic-direct": {APIKey: "sk-test-1234567890"},
		},
	}
	if err := eyriecfg.SaveProviderConfig(cfg, path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	err := SaveRoutingPolicyJSON(`{
  "providers": {
    "anthropic": [{
      "deployments": [{"deployment_id": "anthropic-direct", "weight": 100}],
      "retries": 1
    }]
  }
}`)
	if err != nil {
		t.Fatalf("SaveRoutingPolicyJSON: %v", err)
	}
}

func TestSaveRoutingPolicyJSONRejectsUnknownDeployment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	t.Setenv("HAWK_CONFIG_DIR", dir)
	_ = os.WriteFile(path, []byte(`{"config_version":2}`), 0o600)

	err := SaveRoutingPolicyJSON(`{
  "default": [{
    "deployments": [{"deployment_id": "does-not-exist", "weight": 100}]
  }]
}`)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
