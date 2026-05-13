package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ManifestV2 is the extended manifest format for dynamic plugins.
// It is backward compatible with the original ToolManifest (V1) format.
type ManifestV2 struct {
	// V1 fields (backward compatible)
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description"`
	Author         string         `json:"author"`
	Tools          []ManifestTool `json:"tools"`
	Permissions    []string       `json:"permissions"`
	MinHawkVersion string         `json:"min_hawk_version"`

	// V2 extensions
	Mode         string                 `json:"mode,omitempty"`         // "subprocess" (default) or "daemon"
	Hooks        []ManifestHook         `json:"hooks,omitempty"`        // event hooks
	Config       map[string]interface{} `json:"config,omitempty"`       // plugin configuration
	Dependencies []string               `json:"dependencies,omitempty"` // other plugin names required
	Repository   string                 `json:"repository,omitempty"`   // git repo URL
	License      string                 `json:"license,omitempty"`
	Entrypoint   string                 `json:"entrypoint,omitempty"` // main binary (for daemon mode)
}

// ManifestHook defines an event hook provided by a plugin.
type ManifestHook struct {
	Event    string `json:"event"`              // hook event type (e.g. "pre_tool", "post_query")
	Command  string `json:"command"`            // shell command to run
	Async    bool   `json:"async,omitempty"`    // fire-and-forget
	Priority int    `json:"priority,omitempty"` // lower = earlier (default 100)
}

// ParseManifestV2 reads and parses a plugin.json file from the given plugin directory
// using the V2 manifest format. It is backward compatible with V1 manifests.
func ParseManifestV2(pluginDir string) (*ManifestV2, error) {
	path := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest ManifestV2
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Validate required fields
	if manifest.Name == "" {
		return nil, fmt.Errorf("manifest missing required field: name")
	}
	if manifest.Version == "" {
		return nil, fmt.Errorf("manifest missing required field: version")
	}

	// Set defaults
	if manifest.Mode == "" {
		manifest.Mode = "subprocess"
	}
	for i := range manifest.Hooks {
		if manifest.Hooks[i].Priority == 0 {
			manifest.Hooks[i].Priority = 100
		}
	}

	return &manifest, nil
}

// IsV2 returns true if any V2-specific fields are populated.
func (m *ManifestV2) IsV2() bool {
	if m.Mode != "" && m.Mode != "subprocess" {
		return true
	}
	if len(m.Hooks) > 0 {
		return true
	}
	if len(m.Config) > 0 {
		return true
	}
	if len(m.Dependencies) > 0 {
		return true
	}
	if m.Repository != "" {
		return true
	}
	if m.Entrypoint != "" {
		return true
	}
	if m.License != "" {
		return true
	}
	return false
}

// ToV1 converts a V2 manifest back to the original V1 ToolManifest format.
// V2-only fields are lost in this conversion.
func (m *ManifestV2) ToV1() *ToolManifest {
	return &ToolManifest{
		Name:           m.Name,
		Version:        m.Version,
		Description:    m.Description,
		Author:         m.Author,
		Tools:          m.Tools,
		Permissions:    m.Permissions,
		MinHawkVersion: m.MinHawkVersion,
	}
}

// ValidateV2 performs extended validation on a V2 manifest.
func (m *ManifestV2) ValidateV2() []string {
	var issues []string

	// Basic V1 validation
	if m.Name == "" {
		issues = append(issues, "name is required")
	}
	if m.Version == "" {
		issues = append(issues, "version is required")
	}
	if len(m.Tools) == 0 && len(m.Hooks) == 0 {
		issues = append(issues, "at least one tool or hook is required")
	}

	// V2-specific validation
	if m.Mode != "" && m.Mode != "subprocess" && m.Mode != "daemon" {
		issues = append(issues, fmt.Sprintf("invalid mode %q; must be 'subprocess' or 'daemon'", m.Mode))
	}

	if m.Mode == "daemon" && m.Entrypoint == "" {
		issues = append(issues, "daemon mode requires an entrypoint")
	}

	for i, tool := range m.Tools {
		if tool.Name == "" {
			issues = append(issues, fmt.Sprintf("tools[%d]: name is required", i))
		}
		if tool.Command == "" && m.Mode != "daemon" {
			issues = append(issues, fmt.Sprintf("tools[%d]: command is required for subprocess mode", i))
		}
		if tool.Description == "" {
			issues = append(issues, fmt.Sprintf("tools[%d] (%s): description is recommended", i, tool.Name))
		}
	}

	for i, hook := range m.Hooks {
		if hook.Event == "" {
			issues = append(issues, fmt.Sprintf("hooks[%d]: event is required", i))
		}
		if hook.Command == "" {
			issues = append(issues, fmt.Sprintf("hooks[%d]: command is required", i))
		}
	}

	validPerms := map[string]bool{"network": true, "filesystem": true, "env": true}
	for _, perm := range m.Permissions {
		if !validPerms[perm] {
			issues = append(issues, fmt.Sprintf("unknown permission: %q", perm))
		}
	}

	return issues
}

// WriteManifestV2 writes a ManifestV2 to a plugin directory as plugin.json.
func WriteManifestV2(pluginDir string, m *ManifestV2) error {
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	path := filepath.Join(pluginDir, "plugin.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}
