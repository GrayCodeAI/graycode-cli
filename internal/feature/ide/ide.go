// Package ide provides IDE integration hints and metadata.
// This is a stub for future IDE extensions.
package ide

import (
	"encoding/json"
	"fmt"
)

// ExtensionManifest describes a VSCode/Cursor/Trae extension.
type ExtensionManifest struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	Version     string            `json:"version"`
	Publisher   string            `json:"publisher"`
	Description string            `json:"description"`
	Main        string            `json:"main"`
	Engines     map[string]string `json:"engines"`
	Categories  []string          `json:"categories"`
	Activation  []string          `json:"activationEvents"`
	Contributes *Contributes      `json:"contributes,omitempty"`
}

// Contributes describes extension contributions.
type Contributes struct {
	Commands      []Command      `json:"commands,omitempty"`
	Keybindings   []Keybinding   `json:"keybindings,omitempty"`
	Configuration *Configuration `json:"configuration,omitempty"`
}

// Command describes a contributed command.
type Command struct {
	Command  string `json:"command"`
	Title    string `json:"title"`
	Category string `json:"category,omitempty"`
}

// Keybinding describes a keyboard shortcut.
type Keybinding struct {
	Command string `json:"command"`
	Key     string `json:"key"`
	When    string `json:"when,omitempty"`
}

// Configuration describes contributed settings.
type Configuration struct {
	Title      string                 `json:"title"`
	Properties map[string]interface{} `json:"properties"`
}

// DefaultManifest returns the default Graycode IDE extension manifest.
func DefaultManifest() *ExtensionManifest {
	return &ExtensionManifest{
		Name:        "graycode",
		DisplayName: "Graycode AI Agent",
		Version:     "0.1.0",
		Publisher:   "graycodeai",
		Description: "AI coding agent integration for VS Code",
		Main:        "./out/extension.js",
		Engines: map[string]string{
			"vscode": "^1.74.0",
		},
		Categories: []string{"Machine Learning", "Programming Languages"},
		Activation: []string{"onCommand:graycode.start", "onCommand:graycode.explain", "onCommand:graycode.fix"},
		Contributes: &Contributes{
			Commands: []Command{
				{Command: "graycode.start", Title: "Start Graycode Chat", Category: "Graycode"},
				{Command: "graycode.explain", Title: "Explain Selection", Category: "Graycode"},
				{Command: "graycode.fix", Title: "Fix Selection", Category: "Graycode"},
				{Command: "graycode.review", Title: "Review File", Category: "Graycode"},
				{Command: "graycode.test", Title: "Generate Tests", Category: "Graycode"},
			},
			Keybindings: []Keybinding{
				{Command: "graycode.start", Key: "ctrl+shift+h", When: "editorTextFocus"},
				{Command: "graycode.explain", Key: "ctrl+shift+e", When: "editorHasSelection"},
			},
			Configuration: &Configuration{
				Title: "Graycode",
				Properties: map[string]interface{}{
					"graycode.apiKey": map[string]interface{}{
						"type":        "string",
						"default":     "",
						"description": "API key for the AI provider",
					},
					"graycode.provider": map[string]interface{}{
						"type":        "string",
						"default":     "anthropic",
						"enum":        []string{"anthropic", "openai", "google", "ollama"},
						"description": "AI provider to use",
					},
					"graycode.model": map[string]interface{}{
						"type":        "string",
						"default":     "",
						"description": "Model to use (leave empty for provider default)",
					},
				},
			},
		},
	}
}

// GeneratePackageJSON generates a package.json for the VSCode extension.
func GeneratePackageJSON() ([]byte, error) {
	manifest := DefaultManifest()
	return json.MarshalIndent(manifest, "", "  ")
}

// Hints returns IDE integration hints for the current project.
func Hints() []string {
	return []string{
		"Install the Graycode VSCode extension for inline AI assistance",
		"Use Ctrl+Shift+H to open Graycode chat from VSCode",
		"Use Ctrl+Shift+E to explain selected code",
		"Configure graycode.provider and graycode.model in VSCode settings",
	}
}

// ServerConfig describes a language server configuration.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// SuggestLSPConfig suggests LSP server configurations for common languages.
func SuggestLSPConfig(language string) (*ServerConfig, error) {
	configs := map[string]*ServerConfig{
		"go": {
			Command: "gopls",
			Args:    []string{"serve"},
		},
		"typescript": {
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
		},
		"python": {
			Command: "pylsp",
			Args:    []string{},
		},
		"rust": {
			Command: "rust-analyzer",
			Args:    []string{},
		},
	}

	config, ok := configs[language]
	if !ok {
		return nil, fmt.Errorf("no LSP config for language: %s", language)
	}
	return config, nil
}
