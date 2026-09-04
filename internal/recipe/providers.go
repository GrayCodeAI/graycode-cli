// Package recipe also provides declarative provider configuration.
// This file implements YAML-based provider definitions compatible with eyrie.
package recipe

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
	"gopkg.in/yaml.v3"
)

// ProviderConfig defines an LLM provider declaratively via YAML.
type ProviderConfig struct {
	Name        string            `yaml:"name"`
	DisplayName string            `yaml:"display_name"`
	BaseURL     string            `yaml:"base_url"`
	AuthType    string            `yaml:"auth_type"` // "api_key", "oauth", "none"
	AuthHeader  string            `yaml:"auth_header"`
	AuthPrefix  string            `yaml:"auth_prefix"`
	EnvKey      string            `yaml:"env_key"`
	Models      []ModelDef        `yaml:"models"`
	Format      string            `yaml:"format"` // "openai", "anthropic", "google"
	Headers     map[string]string `yaml:"headers"`
}

// ModelDef defines a model available from a provider.
type ModelDef struct {
	ID          string  `yaml:"id"`
	DisplayName string  `yaml:"display_name"`
	MaxTokens   int     `yaml:"max_tokens"`
	InputPrice  float64 `yaml:"input_price"`
	OutputPrice float64 `yaml:"output_price"`
}

// LoadProviderConfigs reads all YAML provider definitions from a directory.
func LoadProviderConfigs(dir string) ([]ProviderConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var configs []ProviderConfig
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- path is built from a trusted provider config directory listing (os.ReadDir entries), not external input
		if err != nil {
			continue
		}
		var pc ProviderConfig
		if err := yaml.Unmarshal(data, &pc); err != nil {
			continue
		}
		if pc.Name == "" {
			continue
		}
		configs = append(configs, pc)
	}
	return configs, nil
}

// DefaultProviderConfigDirs returns standard directories for provider configs.
func DefaultProviderConfigDirs() []string {
	return []string{
		filepath.Join(storage.StateDir(), "providers"),
		filepath.Join(".agents", "providers"),
	}
}

// Validate checks a provider config for completeness.
func (pc *ProviderConfig) Validate() error {
	if pc.Name == "" {
		return fmt.Errorf("provider missing name")
	}
	if pc.BaseURL == "" {
		return fmt.Errorf("provider %s missing base_url", pc.Name)
	}
	if pc.Format == "" {
		return fmt.Errorf("provider %s missing format", pc.Name)
	}
	return nil
}
