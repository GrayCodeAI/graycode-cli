package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings holds hawk configuration.
type Settings struct {
	Model           string            `json:"model,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	Theme           string            `json:"theme,omitempty"`
	AutoAllow       []string          `json:"auto_allow,omitempty"`       // tools to always allow
	MaxBudgetUSD    float64           `json:"max_budget_usd,omitempty"`   // cost cap per session
	CustomHeaders   map[string]string `json:"custom_headers,omitempty"`
	MCPServers      []MCPServerConfig `json:"mcp_servers,omitempty"`
}

// MCPServerConfig defines an MCP server to connect at startup.
type MCPServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

func globalSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "settings.json")
}

func projectSettingsPath() string {
	return filepath.Join(".hawk", "settings.json")
}

// LoadSettings loads settings from global + project, with project overriding global.
func LoadSettings() Settings {
	var s Settings
	// Global
	if data, err := os.ReadFile(globalSettingsPath()); err == nil {
		json.Unmarshal(data, &s)
	}
	// Project overrides
	var proj Settings
	if data, err := os.ReadFile(projectSettingsPath()); err == nil {
		if json.Unmarshal(data, &proj) == nil {
			if proj.Model != "" {
				s.Model = proj.Model
			}
			if proj.Provider != "" {
				s.Provider = proj.Provider
			}
			if proj.Theme != "" {
				s.Theme = proj.Theme
			}
			if proj.MaxBudgetUSD > 0 {
				s.MaxBudgetUSD = proj.MaxBudgetUSD
			}
			s.AutoAllow = append(s.AutoAllow, proj.AutoAllow...)
			s.MCPServers = append(s.MCPServers, proj.MCPServers...)
		}
	}
	return s
}

// SaveGlobal saves settings to the global config file.
func SaveGlobal(s Settings) error {
	dir := filepath.Dir(globalSettingsPath())
	os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(globalSettingsPath(), data, 0o644)
}

// SaveProject saves settings to the project config file.
func SaveProject(s Settings) error {
	os.MkdirAll(".hawk", 0o755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(projectSettingsPath(), data, 0o644)
}
