package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Distribution defines a custom hawk distribution (white-label configuration).
type Distribution struct {
	Name        string            `yaml:"name"`
	DisplayName string            `yaml:"display_name"`
	Version     string            `yaml:"version"`
	Provider    DistroProvider    `yaml:"provider"`
	Extensions  []DistroExtension `yaml:"extensions"`
	Branding    DistroBranding    `yaml:"branding"`
	Defaults    DistroDefaults    `yaml:"defaults"`
	Recipes     []string          `yaml:"recipes"`
}

// DistroProvider configures the default LLM provider.
type DistroProvider struct {
	Name   string `yaml:"name"`
	Model  string `yaml:"model"`
	EnvKey string `yaml:"env_key"`
}

// DistroExtension defines a bundled extension.
type DistroExtension struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Args    []string `yaml:"args"`
}

// DistroBranding customizes the UI appearance.
type DistroBranding struct {
	Prompt      string `yaml:"prompt"`
	WelcomeMsg  string `yaml:"welcome_message"`
	AgentName   string `yaml:"agent_name"`
	Color       string `yaml:"color"`
}

// DistroDefaults sets default behavior.
type DistroDefaults struct {
	PermissionMode string   `yaml:"permission_mode"`
	AllowedTools   []string `yaml:"allowed_tools"`
	MaxTurns       int      `yaml:"max_turns"`
	SystemPrompt   string   `yaml:"system_prompt"`
}

// LoadDistribution reads a distribution config from a YAML file.
func LoadDistribution(path string) (*Distribution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Distribution
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// FindDistribution looks for a distribution config in standard locations.
func FindDistribution() *Distribution {
	paths := []string{
		"hawk-distro.yaml",
		".hawk/distro.yaml",
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		paths = append(paths, filepath.Join(home, ".hawk", "distro.yaml"))
	}
	for _, p := range paths {
		if d, err := LoadDistribution(p); err == nil {
			return d
		}
	}
	return nil
}
