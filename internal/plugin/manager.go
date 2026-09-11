package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

const maxPluginOutputBytes = 8 << 20

type cappedBuffer struct {
	bytes.Buffer
	maxBytes int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.maxBytes - b.Len()
	if remaining <= 0 {
		return 0, fmt.Errorf("plugin output exceeds %d bytes", b.maxBytes)
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return remaining, fmt.Errorf("plugin output exceeds %d bytes", b.maxBytes)
	}
	return b.Buffer.Write(p)
}

// Plugin represents a loaded plugin with its tools and metadata.
type Plugin struct {
	Name         string
	Version      string
	Description  string
	Author       string
	Tools        []PluginTool
	Path         string
	Manifest     *ToolManifest
	WasmManifest *WasmManifest
	WasmRuntime  *WasmPluginRuntime
}

// PluginTool represents a single tool provided by a plugin.
type PluginTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Command     string
	Timeout     time.Duration
	PluginName  string // namespaced: which plugin owns this tool
	IsWasm      bool   // true if this tool is from a WASM plugin
}

// ToolManifest is the manifest loaded from plugin.json for subprocess-based plugins.
type ToolManifest struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description"`
	Author         string         `json:"author"`
	Tools          []ManifestTool `json:"tools"`
	Permissions    []string       `json:"permissions"`
	MinHawkVersion string         `json:"min_hawk_version"`
}

// WasmManifest is the manifest for WASM-based plugins.
type WasmManifest struct {
	Name           string        `json:"name"`
	Version        string        `json:"version"`
	Description    string        `json:"description"`
	Author         string        `json:"author"`
	Tools          []WasmToolDef `json:"tools"`
	Permissions    []string      `json:"permissions"`
	MinHawkVersion string        `json:"min_hawk_version"`
	WasmPath       string        `json:"wasm_path"` // relative path to .wasm file
}

// WasmToolDef defines a tool in a WASM plugin manifest.
type WasmToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ManifestTool defines a tool in the manifest file.
type ManifestTool struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Command        string                 `json:"command"`
	Args           []string               `json:"args"`
	InputSchema    map[string]interface{} `json:"input_schema"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
}

// PluginManager manages discovery, loading, and execution of subprocess-based plugins.
type PluginManager struct {
	PluginDirs []string
	Loaded     map[string]*Plugin
	mu         sync.RWMutex
}

// SecurityIssue represents a security concern found during plugin scanning.
type SecurityIssue struct {
	Severity string
	Message  string
	File     string
	Line     int
}

// NewPluginManager creates a new PluginManager with the given directories.
// If no directories are provided, defaults to Hawk user state.
func NewPluginManager(dirs ...string) *PluginManager {
	if len(dirs) == 0 {
		dirs = []string{
			filepath.Join(storage.StateDir(), "plugins"),
		}
	}
	return &PluginManager{
		PluginDirs: dirs,
		Loaded:     make(map[string]*Plugin),
	}
}

// Discover walks plugin directories, reads manifests, and returns available plugins.
func (pm *PluginManager) Discover() ([]*Plugin, error) {
	var plugins []*Plugin
	seen := make(map[string]bool)

	for _, dir := range pm.PluginDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read plugin dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginPath := filepath.Join(dir, entry.Name())

			// First try to load as WASM plugin
			wasmManifest, wasmErr := ParseWasmManifest(pluginPath)
			if wasmErr == nil {
				if seen[wasmManifest.Name] {
					continue
				}
				seen[wasmManifest.Name] = true

				p, err := wasmManifestToPlugin(wasmManifest, pluginPath)
				if err != nil {
					continue
				}
				plugins = append(plugins, p)
				continue
			}

			// Fall back to subprocess plugin
			manifest, err := ParseManifest(pluginPath)
			if err != nil {
				continue
			}
			if seen[manifest.Name] {
				continue
			}
			seen[manifest.Name] = true

			p := manifestToPlugin(manifest, pluginPath)
			plugins = append(plugins, p)
		}
	}
	return plugins, nil
}

// Load loads a specific plugin by name from the plugin directories.
func (pm *PluginManager) Load(name string) (*Plugin, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if p, ok := pm.Loaded[name]; ok {
		return p, nil
	}

	for _, dir := range pm.PluginDirs {
		pluginPath := filepath.Join(dir, name)

		// Try WASM first
		wasmManifest, wasmErr := ParseWasmManifest(pluginPath)
		if wasmErr == nil {
			p, err := wasmManifestToPlugin(wasmManifest, pluginPath)
			if err == nil {
				pm.Loaded[name] = p
				return p, nil
			}
		}

		// Fall back to subprocess plugin
		manifest, err := ParseManifest(pluginPath)
		if err != nil {
			continue
		}
		p := manifestToPlugin(manifest, pluginPath)
		pm.Loaded[name] = p
		return p, nil
	}
	return nil, fmt.Errorf("plugin %q not found", name)
}

// LoadAll discovers and loads all available plugins.
func (pm *PluginManager) LoadAll() error {
	plugins, err := pm.Discover()
	if err != nil {
		return err
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, p := range plugins {
		pm.Loaded[p.Name] = p
	}
	return nil
}

// Execute runs a tool from a loaded plugin by passing input as JSON via stdin
// and capturing stdout as the result. It enforces timeouts and captures stderr for errors.
func (pm *PluginManager) Execute(ctx context.Context, pluginName, toolName string, input json.RawMessage) (string, error) {
	pm.mu.RLock()
	p, ok := pm.Loaded[pluginName]
	pm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("plugin %q not loaded", pluginName)
	}

	var tool *PluginTool
	for i := range p.Tools {
		if p.Tools[i].Name == toolName {
			tool = &p.Tools[i]
			break
		}
	}
	if tool == nil {
		return "", fmt.Errorf("tool %q not found in plugin %q", toolName, pluginName)
	}

	// Apply timeout
	timeout := tool.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Handle WASM tools
	if tool.IsWasm && p.WasmRuntime != nil {
		return p.WasmRuntime.ExecuteTool(ctx, toolName, input)
	}

	// Parse command and args (subprocess-based). Quoted arguments are preserved;
	// shell evaluation is intentionally not supported.
	parts := splitCommand(tool.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("tool %q has empty command", toolName)
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...) // #nosec G204 -- tool.Command comes from the plugin's own manifest, trusted like other plugin config
	cmd.Dir = p.Path

	// Pass input via stdin
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}

	var stdout cappedBuffer
	stdout.maxBytes = maxPluginOutputBytes
	var stderr cappedBuffer
	stderr.maxBytes = maxPluginOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("tool %q in plugin %q timed out after %s", toolName, pluginName, timeout)
		}
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("tool %q in plugin %q failed: %s", toolName, pluginName, errMsg)
	}

	return stdout.String(), nil
}

// ListTools returns all tools from all loaded plugins, namespaced by plugin name.
func (pm *PluginManager) ListTools() []PluginTool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var tools []PluginTool
	for _, p := range pm.Loaded {
		tools = append(tools, p.Tools...)
	}
	return tools
}

// Validate checks a manifest for issues and returns a list of warnings/errors.
func Validate(manifest *ToolManifest) []string {
	var issues []string

	if manifest.Name == "" {
		issues = append(issues, "name is required")
	}
	if manifest.Version == "" {
		issues = append(issues, "version is required")
	}
	if len(manifest.Tools) == 0 {
		issues = append(issues, "at least one tool is required")
	}

	for i, tool := range manifest.Tools {
		if tool.Name == "" {
			issues = append(issues, fmt.Sprintf("tools[%d]: name is required", i))
		}
		if tool.Command == "" {
			issues = append(issues, fmt.Sprintf("tools[%d]: command is required", i))
		}
		if tool.Description == "" {
			issues = append(issues, fmt.Sprintf("tools[%d] (%s): description is recommended", i, tool.Name))
		}
	}

	validPerms := map[string]bool{"network": true, "filesystem": true, "env": true}
	for _, perm := range manifest.Permissions {
		if !validPerms[perm] {
			issues = append(issues, fmt.Sprintf("unknown permission: %q", perm))
		}
	}

	return issues
}

// ParseManifest reads and parses a plugin.json file from the given plugin directory.
func ParseManifest(pluginDir string) (*ToolManifest, error) {
	path := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(path) // #nosec G304 -- pluginDir is a locally installed plugin directory under a Hawk-managed plugins root, not raw external input
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest ToolManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &manifest, nil
}

// ScanPlugin checks a plugin directory for suspicious content and security issues.
func ScanPlugin(pluginDir string) []SecurityIssue {
	var issues []SecurityIssue

	// Check manifest for security issues
	manifest, err := ParseManifest(pluginDir)
	if err == nil {
		// Check for overly broad permissions
		if len(manifest.Permissions) >= 3 {
			issues = append(issues, SecurityIssue{
				Severity: "warning",
				Message:  "plugin requests all available permissions",
				File:     filepath.Join(pluginDir, "plugin.json"),
				Line:     0,
			})
		}

		// Check commands for shell injection patterns
		for _, tool := range manifest.Tools {
			if containsShellInjection(tool.Command) {
				issues = append(issues, SecurityIssue{
					Severity: "critical",
					Message:  fmt.Sprintf("tool %q command contains potential shell injection pattern", tool.Name),
					File:     filepath.Join(pluginDir, "plugin.json"),
					Line:     0,
				})
			}
			for _, arg := range tool.Args {
				if containsShellInjection(arg) {
					issues = append(issues, SecurityIssue{
						Severity: "critical",
						Message:  fmt.Sprintf("tool %q arg contains potential shell injection pattern", tool.Name),
						File:     filepath.Join(pluginDir, "plugin.json"),
						Line:     0,
					})
				}
			}

			// Check if tool uses network without declaring permission
			if usesNetwork(tool.Command) && !hasPermission(manifest, "network") {
				issues = append(issues, SecurityIssue{
					Severity: "warning",
					Message:  fmt.Sprintf("tool %q appears to use network but plugin does not declare network permission", tool.Name),
					File:     filepath.Join(pluginDir, "plugin.json"),
					Line:     0,
				})
			}
		}
	}

	// Scan all files in the plugin directory for hidden Unicode characters
	_ = filepath.WalkDir(pluginDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip binary files (check first few bytes)
		fi, fiErr := d.Info()
		if fiErr != nil {
			return fiErr
		}
		if fi.Size() > 10*1024*1024 { // Skip files > 10MB
			return nil
		}

		data, err := os.ReadFile(path) // #nosec G304,G122 -- read-only scan of a locally installed plugin
		if err != nil {
			return nil
		}

		// Check if content appears to be text
		if !isTextContent(data) {
			return nil
		}

		content := string(data)
		lines := strings.Split(content, "\n")
		for lineNum, line := range lines {
			for _, r := range line {
				if isHiddenUnicode(r) {
					issues = append(issues, SecurityIssue{
						Severity: "critical",
						Message:  fmt.Sprintf("hidden Unicode character U+%04X detected", r),
						File:     path,
						Line:     lineNum + 1,
					})
				}
			}
		}
		return nil
	})

	return issues
}

// manifestToPlugin converts a ToolManifest to a Plugin struct.
func manifestToPlugin(manifest *ToolManifest, path string) *Plugin {
	p := &Plugin{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Author:      manifest.Author,
		Path:        path,
		Manifest:    manifest,
	}

	for _, mt := range manifest.Tools {
		timeout := time.Duration(mt.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		command := mt.Command
		if len(mt.Args) > 0 {
			command = command + " " + strings.Join(mt.Args, " ")
		}

		p.Tools = append(p.Tools, PluginTool{
			Name:        mt.Name,
			Description: mt.Description,
			InputSchema: mt.InputSchema,
			Command:     command,
			Timeout:     timeout,
			PluginName:  manifest.Name,
		})
	}

	return p
}

// containsShellInjection checks for common shell injection patterns.
func containsShellInjection(s string) bool {
	patterns := []string{
		"$(", "`", "&&", "||", ";", "|", ">", "<", "eval ", "exec ",
	}
	lower := strings.ToLower(s)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// usesNetwork checks if a command likely accesses the network.
func usesNetwork(cmd string) bool {
	networkCmds := []string{"curl", "wget", "nc", "ncat", "ssh", "http", "fetch"}
	lower := strings.ToLower(cmd)
	for _, nc := range networkCmds {
		if strings.Contains(lower, nc) {
			return true
		}
	}
	return false
}

// hasPermission checks if a manifest declares a specific permission.
func hasPermission(manifest *ToolManifest, perm string) bool {
	for _, p := range manifest.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// isHiddenUnicode checks if a rune is a problematic hidden Unicode character.
func isHiddenUnicode(r rune) bool {
	if r < 128 {
		return false
	}
	// Zero-width characters
	if r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
		return true
	}
	// BiDi override characters
	if r >= 0x202A && r <= 0x202E {
		return true
	}
	// BiDi isolate characters
	if r >= 0x2066 && r <= 0x2069 {
		return true
	}
	// Unicode tag characters
	if r >= 0xE0001 && r <= 0xE007F {
		return true
	}
	// Non-standard control characters
	if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
		return true
	}
	return false
}

// isTextContent performs a basic check to determine if content is likely text.
func isTextContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// Check first 512 bytes for null bytes (binary indicator)
	checkLen := 512
	if len(data) < checkLen {
		checkLen = len(data)
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return false
		}
	}
	return true
}

// ParseWasmManifest reads and parses a plugin.json file for a WASM plugin.
func ParseWasmManifest(pluginDir string) (*WasmManifest, error) {
	path := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(path) // #nosec G304 -- pluginDir is a locally installed plugin directory under a Hawk-managed plugins root, not raw external input
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest WasmManifest
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
	if manifest.WasmPath == "" {
		return nil, fmt.Errorf("wasm plugin missing required field: wasm_path")
	}

	return &manifest, nil
}

// wasmManifestToPlugin converts a WasmManifest to a Plugin struct with WASM runtime.
func wasmManifestToPlugin(manifest *WasmManifest, path string) (*Plugin, error) {
	// The WASM runtime expects the plugin directory path (containing plugin.json)
	wasmRuntime, err := NewWasmPluginRuntime(path)
	if err != nil {
		return nil, fmt.Errorf("create wasm runtime: %w", err)
	}

	p := &Plugin{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Author:       manifest.Author,
		Path:         path,
		WasmManifest: manifest,
		WasmRuntime:  wasmRuntime,
	}

	for _, mt := range manifest.Tools {
		p.Tools = append(p.Tools, PluginTool{
			Name:        mt.Name,
			Description: mt.Description,
			InputSchema: mt.InputSchema,
			Timeout:     30 * time.Second,
			PluginName:  manifest.Name,
			IsWasm:      true,
		})
	}

	return p, nil
}
