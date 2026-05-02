package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hawk/eyrie/catalog"
)

// Settings holds hawk configuration.
// Herm-style: no API keys stored here. Secrets come from environment variables only.
type Settings struct {
	Model             string                 `json:"model,omitempty"`
	Provider          string                 `json:"provider,omitempty"`
	Theme             string                 `json:"theme,omitempty"`
	AutoAllow         []string               `json:"auto_allow,omitempty"`      // tools to always allow
	AllowedTools      []string               `json:"allowedTools,omitempty"`    // archive-compatible allow rules
	DisallowedTools   []string               `json:"disallowedTools,omitempty"` // archive-compatible deny rules
	MaxBudgetUSD      float64                `json:"max_budget_usd,omitempty"`  // cost cap per session
	CustomHeaders     map[string]string       `json:"custom_headers,omitempty"`
	MCPServers        []MCPServerConfig       `json:"mcp_servers,omitempty"`
	CustomProviders   []CustomProviderConfig  `json:"custom_providers,omitempty"`
	RepoMap           bool                    `json:"repo_map,omitempty"`
	RepoMapMaxTokens  int                     `json:"repo_map_max_tokens,omitempty"`
	Sandbox           string                  `json:"sandbox,omitempty"`     // sandbox mode: strict, workspace, off
	AutoCommit        bool                    `json:"auto_commit,omitempty"` // auto-commit file changes
}

// CustomProviderConfig defines a user-specified OpenAI-compatible provider.
type CustomProviderConfig struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	APIKeyEnv string `json:"api_key_env,omitempty"`
	Model     string `json:"model,omitempty"`
}

// UnmarshalJSON accepts both Go-era snake_case keys and archive-style camelCase keys.
func (s *Settings) UnmarshalJSON(data []byte) error {
	type alias Settings
	aux := struct {
		*alias
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
	if len(override.CustomProviders) > 0 {
		base.CustomProviders = append(base.CustomProviders, override.CustomProviders...)
	}
	if override.RepoMap {
		base.RepoMap = true
	}
	if override.RepoMapMaxTokens > 0 {
		base.RepoMapMaxTokens = override.RepoMapMaxTokens
	}
	if override.Sandbox != "" {
		base.Sandbox = override.Sandbox
	}
	if override.AutoCommit {
		base.AutoCommit = true
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
	normalized := normalizeSettingKey(key)
	// Herm-style: API key status comes from environment, not settings file
	if provider, ok := apiKeyProviderFromSettingKey(normalized); ok {
		return EnvKeyStatus(provider), true
	}
	switch normalized {
	case "model":
		return s.Model, true
	case "provider":
		return s.Provider, true
	case "apikey":
		return EnvKeyStatus(s.Provider), true
	case "apikeys":
		return AllEnvKeyStatus(), true
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
// Herm-style: API keys are NOT stored in settings.json. Use environment variables.
func SetGlobalSetting(key, value string) error {
	s := LoadGlobalSettings()
	normalized := normalizeSettingKey(key)
	// Herm-style: reject API key persistence to disk
	if _, ok := apiKeyProviderFromSettingKey(normalized); ok {
		return fmt.Errorf("API keys are not stored in settings.json. Set %s in your environment instead", ProviderAPIKeyEnv(providerFromSettingKey(normalized)))
	}
	if normalized == "apikey" {
		return fmt.Errorf("API keys are not stored in settings.json. Set %s in your environment instead", ProviderAPIKeyEnv(normalizeProviderName(s.Provider)))
	}
	switch normalized {
	case "model":
		s.Model = value
	case "provider":
		s.Provider = value
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

func normalizeProviderName(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	provider = strings.ReplaceAll(provider, "_", "-")
	return provider
}

func apiKeyProviderFromSettingKey(normalized string) (string, bool) {
	for _, prefix := range []string{"apikey.", "apikey:"} {
		if strings.HasPrefix(normalized, prefix) {
			provider := normalizeProviderName(strings.TrimPrefix(normalized, prefix))
			return provider, provider != ""
		}
	}
	return "", false
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

// ─────────────────────────────────────────────────────────────
// Herm-style: API keys from environment only (no disk persistence)
// ─────────────────────────────────────────────────────────────

// ProviderAPIKeyEnv returns the environment variable name for a provider's API key.
func ProviderAPIKeyEnv(provider string) string {
	switch normalizeProviderName(provider) {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini", "google", "gemma":
		return "GEMINI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "canopywave":
		return "CANOPYWAVE_API_KEY"
	case "grok", "xai":
		return "XAI_API_KEY"
	case "opencodego":
		return "OPENCODEGO_API_KEY"
	case "ollama":
		return ""
	default:
		replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_")
		name := strings.ToUpper(replacer.Replace(normalizeProviderName(provider)))
		if name == "" {
			return ""
		}
		return name + "_API_KEY"
	}
}

// EnvKeyStatus returns "set" or "empty" for a provider's API key in the environment.
func EnvKeyStatus(provider string) string {
	envKey := ProviderAPIKeyEnv(provider)
	if envKey == "" {
		return "local"
	}
	if os.Getenv(envKey) != "" {
		return "set"
	}
	return "empty"
}

// AllEnvKeyStatus returns a comma-separated summary of all known API key env vars.
func AllEnvKeyStatus() string {
	providers := []string{
		"anthropic", "openai", "gemini", "openrouter",
		"canopywave", "xai", "opencodego",
	}
	var parts []string
	for _, p := range providers {
		status := EnvKeyStatus(p)
		if status == "set" {
			parts = append(parts, p+":"+status)
		}
	}
	if len(parts) == 0 {
		return "(none set)"
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// LoadAPIKeysFromEnv reads all known API keys from environment variables.
func LoadAPIKeysFromEnv() map[string]string {
	providers := []string{
		"anthropic", "openai", "gemini", "openrouter",
		"canopywave", "xai", "opencodego",
	}
	keys := make(map[string]string)
	for _, p := range providers {
		envKey := ProviderAPIKeyEnv(p)
		if envKey == "" {
			continue
		}
		if v := os.Getenv(envKey); v != "" {
			keys[p] = v
		}
	}
	return keys
}

// APIKeyForProvider reads the API key for a provider from the environment.
func APIKeyForProvider(provider string) string {
	envKey := ProviderAPIKeyEnv(provider)
	if envKey == "" {
		return ""
	}
	return os.Getenv(envKey)
}

// NormalizeProviderForEngine maps hawk provider aliases to eyrie canonical names.
// This is the boundary where hawk names become engine/eyrie names.
func NormalizeProviderForEngine(provider string) string {
	p := normalizeProviderName(provider)
	switch p {
	case "xai":
		return "grok" // eyrie calls it "grok", env var is XAI_API_KEY
	default:
		return p
	}
}

// providerFromSettingKey extracts the provider name from a setting key like "apikey.openai".
func providerFromSettingKey(normalized string) string {
	for _, prefix := range []string{"apikey.", "apikey:"} {
		if strings.HasPrefix(normalized, prefix) {
			return normalizeProviderName(strings.TrimPrefix(normalized, prefix))
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────
// Secure env file for persisting API keys across sessions
// ─────────────────────────────────────────────────────────────

func envFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "env")
}

// LoadEnvFile reads ~/.hawk/env and applies export lines to the process.
func LoadEnvFile() error {
	path := envFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse: export KEY=value
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		rest := strings.TrimPrefix(line, "export ")
		idx := strings.Index(rest, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(rest[:idx])
		value := strings.TrimSpace(rest[idx+1:])
		// Only set if not already set in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return nil
}

// RemoveEnvFile removes an export line from ~/.hawk/env.
func RemoveEnvFile(key string) error {
	path := envFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "export ") {
			lines = append(lines, line)
			continue
		}
		rest := strings.TrimPrefix(line, "export ")
		idx := strings.Index(rest, "=")
		if idx < 0 {
			lines = append(lines, line)
			continue
		}
		existingKey := strings.TrimSpace(rest[:idx])
		if existingKey != key {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return os.Remove(path)
	}
	out := []byte(strings.Join(lines, "\n") + "\n")
	return os.WriteFile(path, out, 0o600)
}

// SaveEnvFile writes an export line to ~/.hawk/env, deduplicating existing entries.
func SaveEnvFile(key, value string) error {
	path := envFilePath()
	os.MkdirAll(filepath.Dir(path), 0o700)

	// Read existing lines, filter out old entries for this key
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "export ") {
				lines = append(lines, line)
				continue
			}
			rest := strings.TrimPrefix(line, "export ")
			idx := strings.Index(rest, "=")
			if idx < 0 {
				lines = append(lines, line)
				continue
			}
			existingKey := strings.TrimSpace(rest[:idx])
			if existingKey != key {
				lines = append(lines, line)
			}
		}
	}

	// Add new entry
	lines = append(lines, fmt.Sprintf("export %s=%s", key, value))

	// Write back with 600 perms
	data := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Live model catalog fetch from eyrie
// ─────────────────────────────────────────────────────────────

// FetchModelsForProvider fetches live models from the provider's API (if key available)
// or returns embedded catalog models. This is the runtime model discovery boundary.
func FetchModelsForProvider(provider string) ([]catalog.ModelCatalogEntry, error) {
	provider = NormalizeProviderForEngine(provider)
	if provider == "" {
		return nil, fmt.Errorf("no provider specified")
	}

	// Build env map for eyrie catalog fetch
	env := make(map[string]string)
	env["ANTHROPIC_API_KEY"] = os.Getenv("ANTHROPIC_API_KEY")
	env["OPENAI_API_KEY"] = os.Getenv("OPENAI_API_KEY")
	env["GEMINI_API_KEY"] = os.Getenv("GEMINI_API_KEY")
	env["OPENROUTER_API_KEY"] = os.Getenv("OPENROUTER_API_KEY")
	env["CANOPYWAVE_API_KEY"] = os.Getenv("CANOPYWAVE_API_KEY")
	env["XAI_API_KEY"] = os.Getenv("XAI_API_KEY")
	env["OPENCODEGO_API_KEY"] = os.Getenv("OPENCODEGO_API_KEY")
	env["OLLAMA_BASE_URL"] = os.Getenv("OLLAMA_BASE_URL")
	env["OPENROUTER_BASE_URL"] = os.Getenv("OPENROUTER_BASE_URL")
	env["CANOPYWAVE_BASE_URL"] = os.Getenv("CANOPYWAVE_BASE_URL")

	// Fetch live catalog from eyrie
	cat, err := catalog.FetchModelCatalog("", env)
	if err != nil {
		// Fallback to embedded catalog
		cat = catalog.LoadModelCatalogSync("")
	}

	models := catalog.ModelsForProvider(&cat, provider)
	if len(models) == 0 {
		return nil, fmt.Errorf("no models found for provider %s", provider)
	}
	return models, nil
}
