package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// ConfigLoadError represents a non-fatal error from loading a single
// config section. The rest of the config is still usable.
type ConfigLoadError struct {
	Section string
	Err     error
}

func (e ConfigLoadError) Error() string {
	return fmt.Sprintf("section %q: %v", e.Section, e.Err)
}

// PartialSettings holds a successfully-loaded Settings plus any
// section-level errors encountered during parsing.
type PartialSettings struct {
	Settings Settings
	Errors   []ConfigLoadError
}

// LoadSettingsPartial attempts to load settings from the given path,
// tolerating failures in individual sections. If the top-level JSON
// parse fails, it returns a zero Settings with a single error.
// If individual sections are malformed, those sections are skipped
// and the rest of the settings are loaded successfully.
func LoadSettingsPartial(path string) PartialSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PartialSettings{}
		}
		return PartialSettings{
			Errors: []ConfigLoadError{{Section: "file", Err: err}},
		}
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return PartialSettings{}
	}

	// First, try full unmarshal (fast path).
	var s Settings
	if err := json.Unmarshal(data, &s); err == nil {
		return PartialSettings{Settings: s}
	}

	// Full unmarshal failed — try section-by-section (slow path).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return PartialSettings{
			Errors: []ConfigLoadError{{Section: "json", Err: fmt.Errorf("invalid JSON: %w", err)}},
		}
	}

	var ps PartialSettings

	// Scalar fields — extract directly.
	ps.Settings.Model = extractString(raw, "model")
	ps.Settings.Provider = extractString(raw, "provider")
	ps.Settings.Theme = extractString(raw, "theme")
	ps.Settings.Sandbox = extractString(raw, "sandbox")
	ps.Settings.MaxBudgetUSD = extractFloat64(raw, "max_budget_usd")
	ps.Settings.RepoMapMaxTokens = extractInt(raw, "repo_map_max_tokens")
	ps.Settings.AutoCompactThresholdPct = extractInt(raw, "auto_compact_threshold_pct")
	ps.Settings.Frugal = extractBool(raw, "frugal")
	ps.Settings.Autonomy = extractInt(raw, "autonomy")

	// Complex fields — unmarshal each section independently.
	loadSection(raw, "auto_allow", &ps.Settings.AutoAllow, &ps.Errors)
	loadSection(raw, "allowedTools", &ps.Settings.AllowedTools, &ps.Errors)
	loadSection(raw, "allowed_tools", &ps.Settings.AllowedTools, &ps.Errors)
	loadSection(raw, "disallowedTools", &ps.Settings.DisallowedTools, &ps.Errors)
	loadSection(raw, "disallowed_tools", &ps.Settings.DisallowedTools, &ps.Errors)
	loadSection(raw, "custom_headers", &ps.Settings.CustomHeaders, &ps.Errors)
	loadSection(raw, "mcp_servers", &ps.Settings.MCPServers, &ps.Errors)
	loadSection(raw, "custom_providers", &ps.Settings.CustomProviders, &ps.Errors)
	loadSection(raw, "model_roles", &ps.Settings.ModelRoles, &ps.Errors)
	loadSection(raw, "attribution", &ps.Settings.Attribution, &ps.Errors)
	loadSection(raw, "auto_commit", &ps.Settings.AutoCommit, &ps.Errors)
	loadSection(raw, "deployment_routing", &ps.Settings.DeploymentRouting, &ps.Errors)
	loadSection(raw, "repo_map", &ps.Settings.RepoMap, &ps.Errors)

	return ps
}

// loadSection attempts to unmarshal a single section from the raw JSON map.
// On failure, it appends to the errors slice.
func loadSection(raw map[string]json.RawMessage, key string, dest interface{}, errors *[]ConfigLoadError) {
	data, ok := raw[key]
	if !ok {
		return // section not present — not an error
	}
	if err := json.Unmarshal(data, dest); err != nil {
		*errors = append(*errors, ConfigLoadError{
			Section: key,
			Err:     err,
		})
	}
}

func extractString(raw map[string]json.RawMessage, key string) string {
	data, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s
}

func extractFloat64(raw map[string]json.RawMessage, key string) float64 {
	data, ok := raw[key]
	if !ok {
		return 0
	}
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return 0
	}
	return f
}

func extractInt(raw map[string]json.RawMessage, key string) int {
	data, ok := raw[key]
	if !ok {
		return 0
	}
	var i int
	if err := json.Unmarshal(data, &i); err != nil {
		return 0
	}
	return i
}

func extractBool(raw map[string]json.RawMessage, key string) bool {
	data, ok := raw[key]
	if !ok {
		return false
	}
	var b bool
	if err := json.Unmarshal(data, &b); err != nil {
		return false
	}
	return b
}
