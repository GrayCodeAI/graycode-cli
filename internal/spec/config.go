package spec

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SpecConfig stores user preferences for spec-driven development.
// Fields marked empty ("") mean "AI decides".
type SpecConfig struct {
	Language      string `yaml:"language,omitempty" json:"language,omitempty"`
	Framework     string `yaml:"framework,omitempty" json:"framework,omitempty"`
	Methodology   string `yaml:"methodology,omitempty" json:"methodology,omitempty"`
	Architecture  string `yaml:"architecture,omitempty" json:"architecture,omitempty"`
	RepoStructure string `yaml:"repo_structure,omitempty" json:"repo_structure,omitempty"`
	CustomPrompt  string `yaml:"custom_prompt,omitempty" json:"custom_prompt,omitempty"`
}

// IsEmpty returns true if no fields are set.
func (sc SpecConfig) IsEmpty() bool {
	return sc.Language == "" && sc.Framework == "" &&
		sc.Methodology == "" && sc.Architecture == "" &&
		sc.RepoStructure == "" && sc.CustomPrompt == ""
}

// HasAIDecide returns true if at least one field is explicitly "ai" meaning the AI should decide.
func (sc SpecConfig) HasAIDecide() bool {
	return sc.Language == "ai" || sc.Framework == "ai" ||
		sc.Methodology == "ai" || sc.Architecture == "ai" ||
		sc.RepoStructure == "ai"
}

// Format returns a human-readable summary for display.
func (sc SpecConfig) Format() string {
	if sc.IsEmpty() {
		return "No config set. AI will infer everything from the codebase."
	}
	var s string
	s += fmt.Sprintf("  Language:      %s\n", valOrAIDecide(sc.Language))
	s += fmt.Sprintf("  Framework:     %s\n", valOrAIDecide(sc.Framework))
	s += fmt.Sprintf("  Methodology:   %s\n", valOrAIDecide(sc.Methodology))
	s += fmt.Sprintf("  Architecture:  %s\n", valOrAIDecide(sc.Architecture))
	s += fmt.Sprintf("  Repo structure: %s\n", valOrAIDecide(sc.RepoStructure))
	if sc.CustomPrompt != "" {
		s += fmt.Sprintf("  Custom prompt: %s\n", sc.CustomPrompt)
	}
	return s
}

func valOrAIDecide(v string) string {
	if v == "" || v == "ai" {
		return "AI decides"
	}
	return v
}

// FormatForPrompt returns config formatted for injection into the system prompt.
func (sc SpecConfig) FormatForPrompt() string {
	if sc.IsEmpty() {
		return ""
	}
	var s string
	s += "\n\n## Spec Configuration (user preferences)\n"
	if sc.Language != "" && sc.Language != "ai" {
		s += fmt.Sprintf("- Language: %s\n", sc.Language)
	}
	if sc.Framework != "" && sc.Framework != "ai" {
		s += fmt.Sprintf("- Framework: %s\n", sc.Framework)
	}
	if sc.Methodology != "" && sc.Methodology != "ai" {
		s += fmt.Sprintf("- Methodology: %s\n", sc.Methodology)
	}
	if sc.Architecture != "" && sc.Architecture != "ai" {
		s += fmt.Sprintf("- Architecture: %s\n", sc.Architecture)
	}
	if sc.RepoStructure != "" && sc.RepoStructure != "ai" {
		s += fmt.Sprintf("- Repo structure: %s\n", sc.RepoStructure)
	}
	if sc.CustomPrompt != "" {
		s += fmt.Sprintf("- Custom instructions: %s\n", sc.CustomPrompt)
	}
	s += "Respect these preferences when writing specs, plans, and tasks."
	return s
}

// SpecConfigPath returns the path to the spec config file.
func SpecConfigPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cwd, ".hawk")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "spec-config.yaml"), nil
}

// LoadSpecConfig reads the spec config from disk. Missing file = empty config.
func LoadSpecConfig() SpecConfig {
	path, err := SpecConfigPath()
	if err != nil {
		return SpecConfig{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SpecConfig{}
	}
	var sc SpecConfig
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return SpecConfig{}
	}
	return sc
}

// SaveSpecConfig writes the spec config to disk.
func SaveSpecConfig(sc SpecConfig) error {
	path, err := SpecConfigPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(&sc)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// SpecConfigField describes a single field in the config.
type SpecConfigField struct {
	Key      string
	Label    string
	Help     string
	Examples []string
}

// SpecConfigFields returns the list of configurable fields with metadata.
func SpecConfigFields() []SpecConfigField {
	return []SpecConfigField{
		{Key: "language", Label: "Language", Help: "Go, Python, TypeScript, Rust, etc. Leave empty or 'ai' for AI to decide.", Examples: []string{"Go", "Python", "TypeScript", "Rust", "ai"}},
		{Key: "framework", Label: "Framework", Help: "React, Gin, Django, Next.js, etc. Leave empty or 'ai' for AI to decide.", Examples: []string{"React", "Gin", "Django", "Next.js", "ai"}},
		{Key: "methodology", Label: "Methodology", Help: "Development process style.", Examples: []string{"agile", "waterfall", "kanban", "scrum", "ai"}},
		{Key: "architecture", Label: "Architecture", Help: "Overall system architecture pattern.", Examples: []string{"monolith", "monorepo", "microservices", "clean architecture", "ai"}},
		{Key: "repo_structure", Label: "Repo Structure", Help: "How the codebase organizes code.", Examples: []string{"flat", "modular", "layered", "feature-based", "domain-driven", "ai"}},
		{Key: "custom_prompt", Label: "Custom Prompt", Help: "Any additional instructions for the AI about how to write specs and code."},
	}
}
