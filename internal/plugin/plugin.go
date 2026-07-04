package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Manifest defines a hawk plugin.
type Manifest struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Description string       `json:"description,omitempty"`
	Author      string       `json:"author,omitempty"`
	Commands    []CommandDef `json:"commands,omitempty"`
	Skills      []string     `json:"skills,omitempty"`
	Hooks       []HookDef    `json:"hooks,omitempty"`
	Bridge      *BridgeDef   `json:"bridge,omitempty"`
}

// BridgeDef defines a shell-based bridge plugin that wraps an external CLI binary.
// This lets community plugins integrate CLI tools without writing Go code.
type BridgeDef struct {
	Command string            `json:"command"`        // executable name or path
	Args    []string          `json:"args,omitempty"` // default arguments prepended to each call
	Env     map[string]string `json:"env,omitempty"`  // extra environment variables
}

// CommandDef defines a plugin-provided command.
type CommandDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Script      string `json:"script,omitempty"`
}

// HookDef defines a plugin hook.
type HookDef struct {
	Event   string `json:"event"`
	Command string `json:"command"`
}

// Validate checks if a manifest is valid.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("plugin version is required")
	}
	for _, cmd := range m.Commands {
		if cmd.Name == "" {
			return fmt.Errorf("command name is required")
		}
	}
	if m.Bridge != nil && m.Bridge.Command == "" {
		return fmt.Errorf("bridge command is required when bridge is set")
	}
	return nil
}

func pluginsDir() string {
	return filepath.Join(storage.StateDir(), "plugins")
}

// LoadManifest loads a plugin manifest from a directory.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin.json: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse plugin.json: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns all installed plugins.
func List() ([]*Manifest, error) {
	dir := pluginsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := LoadManifest(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// Install installs a plugin from a directory.
func Install(srcDir string) error {
	m, err := LoadManifest(srcDir)
	if err != nil {
		return err
	}
	issues := criticalPluginIssues(ScanPlugin(srcDir))
	issues = append(issues, criticalManifestIssues(m)...)
	if len(issues) > 0 {
		return fmt.Errorf("plugin security scan failed: %s", strings.Join(issues, "; "))
	}
	dstDir := filepath.Join(pluginsDir(), m.Name)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	// Copy manifest
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dstDir, "plugin.json"), data, 0o644); err != nil {
		return err
	}
	return nil
}

func criticalPluginIssues(issues []SecurityIssue) []string {
	var out []string
	for _, issue := range issues {
		if strings.EqualFold(issue.Severity, "critical") {
			out = append(out, issue.Message)
		}
	}
	return out
}

func criticalManifestIssues(m *Manifest) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, cmd := range m.Commands {
		if containsShellInjection(cmd.Script) {
			out = append(out, fmt.Sprintf("command %q script contains potential shell injection pattern", cmd.Name))
		}
	}
	for _, hook := range m.Hooks {
		if containsShellInjection(hook.Command) {
			out = append(out, fmt.Sprintf("hook %q command contains potential shell injection pattern", hook.Event))
		}
	}
	return out
}

// Uninstall removes a plugin.
func Uninstall(name string) error {
	dir := filepath.Join(pluginsDir(), name)
	return os.RemoveAll(dir)
}

// Summary returns a formatted summary of installed plugins.
func Summary() string {
	plugins, err := List()
	if err != nil || len(plugins) == 0 {
		return "No plugins installed."
	}
	var out string
	for _, p := range plugins {
		out += fmt.Sprintf("%s@%s", p.Name, p.Version)
		if p.Description != "" {
			out += " - " + p.Description
		}
		out += "\n"
	}
	return out
}
