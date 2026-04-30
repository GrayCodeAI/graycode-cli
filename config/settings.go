package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Settings holds hawk configuration.
type Settings struct {
	Model           string            `json:"model,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	Theme           string            `json:"theme,omitempty"`
	AutoAllow       []string          `json:"auto_allow,omitempty"`      // tools to always allow
	AllowedTools    []string          `json:"allowedTools,omitempty"`    // archive-compatible allow rules
	DisallowedTools []string          `json:"disallowedTools,omitempty"` // archive-compatible deny rules
	MaxBudgetUSD    float64           `json:"max_budget_usd,omitempty"`  // cost cap per session
	CustomHeaders   map[string]string `json:"custom_headers,omitempty"`
	MCPServers      []MCPServerConfig `json:"mcp_servers,omitempty"`
}

// UnmarshalJSON accepts both Go-era snake_case keys and archive-style camelCase keys.
func (s *Settings) UnmarshalJSON(data []byte) error {
	type alias Settings
	aux := struct {
		*alias
		APIKeyCamel          string            `json:"apiKey,omitempty"`
		AutoAllowCamel       []string          `json:"autoAllow,omitempty"`
		MaxBudgetUSDCamel    float64           `json:"maxBudgetUSD,omitempty"`
		CustomHeadersCamel   map[string]string `json:"customHeaders,omitempty"`
		MCPServersCamel      []MCPServerConfig `json:"mcpServers,omitempty"`
		AllowedToolsSnake    []string          `json:"allowed_tools,omitempty"`
		DisallowedToolsSnake []string          `json:"disallowed_tools,omitempty"`
	}{
		alias: (*alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if s.APIKey == "" {
		s.APIKey = aux.APIKeyCamel
	}
	if len(s.AutoAllow) == 0 {
		s.AutoAllow = aux.AutoAllowCamel
	}
	if s.MaxBudgetUSD == 0 {
		s.MaxBudgetUSD = aux.MaxBudgetUSDCamel
	}
	if len(s.CustomHeaders) == 0 {
		s.CustomHeaders = aux.CustomHeadersCamel
	}
	if len(s.MCPServers) == 0 {
		s.MCPServers = aux.MCPServersCamel
	}
	if len(s.AllowedTools) == 0 {
		s.AllowedTools = aux.AllowedToolsSnake
	}
	if len(s.DisallowedTools) == 0 {
		s.DisallowedTools = aux.DisallowedToolsSnake
	}
	return nil
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

// LoadGlobalSettings loads only ~/.hawk/settings.json.
func LoadGlobalSettings() Settings {
	var s Settings
	if data, err := os.ReadFile(globalSettingsPath()); err == nil {
		json.Unmarshal(data, &s)
	}
	return s
}

// LoadSettings loads settings from global + project, with project overriding global.
func LoadSettings() Settings {
	s := LoadGlobalSettings()
	// Project overrides
	var proj Settings
	if data, err := os.ReadFile(projectSettingsPath()); err == nil {
		if json.Unmarshal(data, &proj) == nil {
			s = MergeSettings(s, proj)
		}
	}
	return s
}

// LoadSettingsWithOverride loads normal settings plus a JSON object or JSON file override.
func LoadSettingsWithOverride(override string) (Settings, error) {
	s := LoadSettings()
	if override == "" {
		return s, nil
	}
	var extra Settings
	if err := readSettingsOverride(override, &extra); err != nil {
		return s, err
	}
	return MergeSettings(s, extra), nil
}

func readSettingsOverride(source string, out *Settings) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	if strings.HasPrefix(source, "{") {
		return json.Unmarshal([]byte(source), out)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// MergeSettings applies override fields on top of base using project-style precedence.
func MergeSettings(base, override Settings) Settings {
	if override.Model != "" {
		base.Model = override.Model
	}
	if override.Provider != "" {
		base.Provider = override.Provider
	}
	if override.APIKey != "" {
		base.APIKey = override.APIKey
	}
	if override.Theme != "" {
		base.Theme = override.Theme
	}
	if override.MaxBudgetUSD > 0 {
		base.MaxBudgetUSD = override.MaxBudgetUSD
	}
	if len(override.AutoAllow) > 0 {
		base.AutoAllow = append(base.AutoAllow, override.AutoAllow...)
	}
	if len(override.AllowedTools) > 0 {
		base.AllowedTools = append(base.AllowedTools, override.AllowedTools...)
	}
	if len(override.DisallowedTools) > 0 {
		base.DisallowedTools = append(base.DisallowedTools, override.DisallowedTools...)
	}
	if len(override.MCPServers) > 0 {
		base.MCPServers = append(base.MCPServers, override.MCPServers...)
	}
	if len(override.CustomHeaders) > 0 {
		if base.CustomHeaders == nil {
			base.CustomHeaders = make(map[string]string, len(override.CustomHeaders))
		}
		for k, v := range override.CustomHeaders {
			base.CustomHeaders[k] = v
		}
	}
	return base
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

// SettingValue returns a display-safe value for a supported setting key.
func SettingValue(s Settings, key string) (string, bool) {
	switch normalizeSettingKey(key) {
	case "model":
		return s.Model, true
	case "provider":
		return s.Provider, true
	case "apikey":
		if s.APIKey == "" {
			return "", true
		}
		return "(set)", true
	case "theme":
		return s.Theme, true
	case "autoallow":
		return strings.Join(s.AutoAllow, ", "), true
	case "allowedtools":
		return strings.Join(s.AllowedTools, ", "), true
	case "disallowedtools":
		return strings.Join(s.DisallowedTools, ", "), true
	case "maxbudgetusd":
		if s.MaxBudgetUSD == 0 {
			return "", true
		}
		return strconv.FormatFloat(s.MaxBudgetUSD, 'f', -1, 64), true
	case "customheaders":
		data, _ := json.Marshal(s.CustomHeaders)
		return string(data), true
	case "mcpservers":
		data, _ := json.Marshal(s.MCPServers)
		return string(data), true
	default:
		return "", false
	}
}

// SetGlobalSetting updates a supported scalar/list setting in ~/.hawk/settings.json.
func SetGlobalSetting(key, value string) error {
	s := LoadGlobalSettings()
	switch normalizeSettingKey(key) {
	case "model":
		s.Model = value
	case "provider":
		s.Provider = value
	case "apikey":
		s.APIKey = value
	case "theme":
		s.Theme = value
	case "autoallow":
		s.AutoAllow = splitSettingList(value)
	case "allowedtools":
		s.AllowedTools = splitSettingList(value)
	case "disallowedtools":
		s.DisallowedTools = splitSettingList(value)
	case "maxbudgetusd":
		amount, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid max budget: %w", err)
		}
		s.MaxBudgetUSD = amount
	default:
		return fmt.Errorf("unsupported setting key %q", key)
	}
	return SaveGlobal(s)
}

func normalizeSettingKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func splitSettingList(value string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// LoadAPIKeyFromSettings loads the saved API key into the environment if not already set.
func LoadAPIKeyFromSettings() {
	s := LoadSettings()
	ApplyAPIKeyFromSettings(s)
}

// ApplyAPIKeyFromSettings loads a provider API key into the process environment.
func ApplyAPIKeyFromSettings(s Settings) {
	if s.APIKey == "" || s.Provider == "" {
		return
	}
	envKeys := map[string]string{
		"anthropic":  "ANTHROPIC_API_KEY",
		"openai":     "OPENAI_API_KEY",
		"gemini":     "GEMINI_API_KEY",
		"openrouter": "OPENROUTER_API_KEY",
		"groq":       "GROQ_API_KEY",
		"grok":       "XAI_API_KEY",
	}
	if envKey, ok := envKeys[s.Provider]; ok {
		if os.Getenv(envKey) == "" {
			os.Setenv(envKey, s.APIKey)
		}
	}
}
