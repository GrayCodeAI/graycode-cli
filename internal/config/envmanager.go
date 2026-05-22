package config

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

// EnvVar represents a single environment variable with metadata.
type EnvVar struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Source      string `json:"source"` // "env", "file", "profile", "default"
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// EnvManager manages environment variables, profiles, and secrets.
type EnvManager struct {
	Vars          map[string]*EnvVar  `json:"vars"`
	Profiles      map[string][]string `json:"profiles"`
	ActiveProfile string              `json:"active_profile"`
	mu            sync.RWMutex
}

// NewEnvManager creates a new EnvManager with initialized maps.
func NewEnvManager() *EnvManager {
	return &EnvManager{
		Vars:     make(map[string]*EnvVar),
		Profiles: make(map[string][]string),
	}
}

// Load reads environment variables from explicit file sources when provided.
// By default only the OS environment is used — API keys are not loaded from .env files.
func (em *EnvManager) Load(sources ...string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Only load from files when callers pass explicit paths (tests/tools).
	fileSources := sources

	// Load from files in order (lowest priority first)
	for _, src := range fileSources {
		parsed, err := em.parseEnvFileInternal(src)
		if err != nil {
			// File not existing is not an error
			if os.IsNotExist(err) {
				continue
			}
			continue
		}
		sourceName := sourceNameFromPath(src)
		for key, value := range parsed {
			if existing, ok := em.Vars[key]; ok {
				// Higher priority sources are loaded later, so overwrite
				existing.Value = value
				existing.Source = sourceName
			} else {
				em.Vars[key] = &EnvVar{
					Key:    key,
					Value:  value,
					Source: sourceName,
				}
			}
		}
	}

	// OS environment has highest priority — overwrite anything loaded from files
	for key, ev := range em.Vars {
		if osVal := os.Getenv(key); osVal != "" {
			ev.Value = osVal
			ev.Source = "env"
		}
		_ = ev
		_ = key
	}

	return nil
}

// sourceNameFromPath returns a human-readable source name for a file path.
func sourceNameFromPath(path string) string {
	base := filepath.Base(path)
	switch base {
	case ".env":
		return ".env"
	case ".env.local":
		return ".env.local"
	default:
		return "file"
	}
}

// Get returns the value of an environment variable, or empty string if not found.
func (em *EnvManager) Get(key string) string {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if ev, ok := em.Vars[key]; ok {
		return ev.Value
	}
	// Fallback to OS environment
	return os.Getenv(key)
}

// GetRequired returns the value of a required environment variable.
// Returns an error if the variable is not set or is empty.
func (em *EnvManager) GetRequired(key string) (string, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if ev, ok := em.Vars[key]; ok && ev.Value != "" {
		return ev.Value, nil
	}
	// Try OS environment as fallback
	if val := os.Getenv(key); val != "" {
		return val, nil
	}
	return "", fmt.Errorf("required environment variable %q is not set", key)
}

// Set sets an environment variable with the given key and value.
// If secret is true, the value will be masked in display output.
func (em *EnvManager) Set(key, value string, secret bool) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if ev, ok := em.Vars[key]; ok {
		ev.Value = value
		ev.Secret = secret
		ev.Source = "profile"
	} else {
		em.Vars[key] = &EnvVar{
			Key:    key,
			Value:  value,
			Source: "profile",
			Secret: secret,
		}
	}
}

// ParseEnvFile parses a .env file and returns a map of key-value pairs.
// Supports: KEY=value, KEY="quoted value", KEY='single quoted',
// comments (#), empty lines, export prefix, and multiline values with \.
func ParseEnvFile(path string) (map[string]string, error) {
	em := &EnvManager{}
	return em.parseEnvFileInternal(path)
}

func (em *EnvManager) parseEnvFileInternal(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)

	var multilineKey string
	var multilineValue strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Handle multiline continuation
		if multilineKey != "" {
			if strings.HasSuffix(line, "\\") {
				multilineValue.WriteString(strings.TrimSuffix(line, "\\"))
				multilineValue.WriteString("\n")
				continue
			}
			multilineValue.WriteString(line)
			result[multilineKey] = multilineValue.String()
			multilineKey = ""
			multilineValue.Reset()
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}

		// Strip optional "export " prefix
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimPrefix(trimmed, "export ")
			trimmed = strings.TrimSpace(trimmed)
		}

		// Parse KEY=VALUE
		eqIdx := strings.IndexByte(trimmed, '=')
		if eqIdx < 0 {
			continue
		}

		key := strings.TrimSpace(trimmed[:eqIdx])
		value := strings.TrimSpace(trimmed[eqIdx+1:])

		// Check for multiline with trailing backslash
		if strings.HasSuffix(value, "\\") {
			multilineKey = key
			multilineValue.Reset()
			multilineValue.WriteString(strings.TrimSuffix(value, "\\"))
			multilineValue.WriteString("\n")
			continue
		}

		// Remove surrounding quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		result[key] = value
	}

	// Handle case where file ends during multiline
	if multilineKey != "" {
		result[multilineKey] = multilineValue.String()
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// MaskSecrets replaces known secret values in the given text with "***".
func (em *EnvManager) MaskSecrets(text string) string {
	em.mu.RLock()
	defer em.mu.RUnlock()

	for _, ev := range em.Vars {
		if ev.Secret && ev.Value != "" {
			text = strings.ReplaceAll(text, ev.Value, "***")
		}
	}
	return text
}

// ListForDisplay returns a formatted string showing all environment variables.
// Secret values are partially masked; the source is displayed for each variable.
func (em *EnvManager) ListForDisplay() string {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if len(em.Vars) == 0 {
		return "Environment Variables:\n  (none)"
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(em.Vars))
	for k := range em.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Find max key length for alignment
	maxKeyLen := 0
	for _, k := range keys {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
	}

	var b strings.Builder
	b.WriteString("Environment Variables:\n")

	for _, key := range keys {
		ev := em.Vars[key]
		displayValue := ev.Value

		if ev.Secret {
			displayValue = maskValue(ev.Value)
		}

		padding := strings.Repeat(" ", maxKeyLen-len(key))
		meta := formatMeta(ev)
		b.WriteString(fmt.Sprintf("  %s%s = %s %s\n", key, padding, displayValue, meta))
	}

	return b.String()
}

// maskValue partially masks a secret value, showing first few and last 2 characters.
func maskValue(value string) string {
	if len(value) <= 4 {
		return "***"
	}
	if len(value) <= 8 {
		return value[:2] + "***" + value[len(value)-2:]
	}
	// Show prefix (up to 6 chars) and last 2 chars
	prefixLen := 6
	if prefixLen > len(value)/3 {
		prefixLen = len(value) / 3
	}
	return value[:prefixLen] + "***..." + "***" + value[len(value)-2:]
}

// formatMeta returns the metadata annotation string for display.
func formatMeta(ev *EnvVar) string {
	parts := []string{}
	if ev.Secret {
		parts = append(parts, "secret")
	}
	parts = append(parts, "from: "+ev.Source)
	return "(" + strings.Join(parts, ", ") + ")"
}

// Validate checks that all required variables are set and returns warnings
// for missing optional but recommended variables.
func (em *EnvManager) Validate() []string {
	em.mu.RLock()
	defer em.mu.RUnlock()

	var warnings []string

	for key, ev := range em.Vars {
		if ev.Required && ev.Value == "" {
			warnings = append(warnings, fmt.Sprintf("ERROR: required variable %q is not set", key))
		}
	}

	// Recommended provider credentials live in the OS secret store.
	recommended := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY"}
	ctx := context.Background()
	for _, key := range recommended {
		if _, ok := em.Vars[key]; !ok {
			if !eyrieclient.HasSecret(ctx, key) {
				warnings = append(warnings, fmt.Sprintf("WARNING: recommended credential %q is not configured — run /config", key))
			}
		}
	}

	sort.Strings(warnings)
	return warnings
}

// SaveProfile saves a named profile with the given list of variable keys.
func (em *EnvManager) SaveProfile(name string, vars []string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	em.Profiles[name] = make([]string, len(vars))
	copy(em.Profiles[name], vars)
	return nil
}

// LoadProfile activates a named profile, setting its variables as active.
func (em *EnvManager) LoadProfile(name string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	vars, ok := em.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	em.ActiveProfile = name

	// Mark variables that belong to this profile
	for _, key := range vars {
		if ev, exists := em.Vars[key]; exists {
			ev.Source = "profile"
		}
	}

	return nil
}

// Diff returns a list of differences between this EnvManager and another.
func (em *EnvManager) Diff(other *EnvManager) []string {
	em.mu.RLock()
	defer em.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	var diffs []string

	// Find keys in em but not in other, or with different values
	allKeys := make(map[string]bool)
	for k := range em.Vars {
		allKeys[k] = true
	}
	for k := range other.Vars {
		allKeys[k] = true
	}

	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		evA, inA := em.Vars[key]
		evB, inB := other.Vars[key]

		switch {
		case inA && !inB:
			diffs = append(diffs, fmt.Sprintf("+ %s=%s (source: %s)", key, safeDisplay(evA), evA.Source))
		case !inA && inB:
			diffs = append(diffs, fmt.Sprintf("- %s=%s (source: %s)", key, safeDisplay(evB), evB.Source))
		case inA && inB && evA.Value != evB.Value:
			diffs = append(diffs, fmt.Sprintf("~ %s: %s -> %s", key, safeDisplay(evA), safeDisplay(evB)))
		}
	}

	return diffs
}

// safeDisplay returns a display-safe value for an EnvVar.
func safeDisplay(ev *EnvVar) string {
	if ev.Secret {
		return maskValue(ev.Value)
	}
	return ev.Value
}

// Export returns the environment variables in the specified format.
// Supported formats: "env" (KEY=value), "json" (JSON object), "shell" (export KEY=value).
func (em *EnvManager) Export(format string) string {
	em.mu.RLock()
	defer em.mu.RUnlock()

	keys := make([]string, 0, len(em.Vars))
	for k := range em.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	switch format {
	case "json":
		obj := make(map[string]string, len(em.Vars))
		for _, key := range keys {
			obj[key] = em.Vars[key].Value
		}
		data, _ := json.MarshalIndent(obj, "", "  ")
		return string(data)

	case "shell":
		var b strings.Builder
		for _, key := range keys {
			b.WriteString(fmt.Sprintf("export %s=%s\n", key, shellQuote(em.Vars[key].Value)))
		}
		return b.String()

	default: // "env"
		var b strings.Builder
		for _, key := range keys {
			b.WriteString(fmt.Sprintf("%s=%s\n", key, em.Vars[key].Value))
		}
		return b.String()
	}
}

// shellQuote wraps a value in double quotes if it contains spaces or special characters.
func shellQuote(value string) string {
	if value == "" {
		return `""`
	}
	needsQuoting := false
	for _, c := range value {
		if c == ' ' || c == '\t' || c == '"' || c == '\'' || c == '\\' ||
			c == '$' || c == '`' || c == '!' || c == '#' || c == '&' ||
			c == '|' || c == ';' || c == '(' || c == ')' {
			needsQuoting = true
			break
		}
	}
	if !needsQuoting {
		return value
	}
	// Escape special characters inside double quotes
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, `$`, `\$`)
	escaped = strings.ReplaceAll(escaped, "`", "\\`")
	return `"` + escaped + `"`
}
