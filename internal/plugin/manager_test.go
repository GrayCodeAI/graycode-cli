package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

func TestParseManifestValid(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"name": "test-plugin",
		"version": "1.0.0",
		"description": "A test plugin",
		"author": "Test Author",
		"tools": [
			{
				"name": "greet",
				"description": "Says hello",
				"command": "echo",
				"args": ["hello"],
				"input_schema": {"type": "object", "properties": {"name": {"type": "string"}}},
				"timeout_seconds": 10
			}
		],
		"permissions": ["filesystem"],
		"min_hawk_version": "0.1.0"
	}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseManifest(dir)
	if err != nil {
		t.Fatalf("ParseManifest() error: %v", err)
	}

	if m.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", m.Version)
	}
	if m.Description != "A test plugin" {
		t.Errorf("expected description 'A test plugin', got %q", m.Description)
	}
	if m.Author != "Test Author" {
		t.Errorf("expected author 'Test Author', got %q", m.Author)
	}
	if len(m.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(m.Tools))
	}
	if m.Tools[0].Name != "greet" {
		t.Errorf("expected tool name 'greet', got %q", m.Tools[0].Name)
	}
	if m.Tools[0].Command != "echo" {
		t.Errorf("expected tool command 'echo', got %q", m.Tools[0].Command)
	}
	if len(m.Tools[0].Args) != 1 || m.Tools[0].Args[0] != "hello" {
		t.Errorf("expected tool args ['hello'], got %v", m.Tools[0].Args)
	}
	if m.Tools[0].TimeoutSeconds != 10 {
		t.Errorf("expected timeout 10, got %d", m.Tools[0].TimeoutSeconds)
	}
	if len(m.Permissions) != 1 || m.Permissions[0] != "filesystem" {
		t.Errorf("expected permissions ['filesystem'], got %v", m.Permissions)
	}
	if m.MinHawkVersion != "0.1.0" {
		t.Errorf("expected min_hawk_version '0.1.0', got %q", m.MinHawkVersion)
	}
}

func TestParseManifestInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseManifest(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseManifestMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := ParseManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing plugin.json, got nil")
	}
}

func TestPluginDiscovery(t *testing.T) {
	baseDir := t.TempDir()

	// Create two plugin directories
	for _, name := range []string{"plugin-a", "plugin-b"} {
		pluginDir := filepath.Join(baseDir, name)
		os.MkdirAll(pluginDir, 0o755)
		manifest := map[string]interface{}{
			"name":    name,
			"version": "1.0.0",
			"tools": []map[string]interface{}{
				{
					"name":    "tool1",
					"command": "echo",
					"args":    []string{"hi"},
				},
			},
		}
		data, _ := json.Marshal(manifest)
		os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644)
	}

	pm := NewPluginManager(baseDir)
	plugins, err := pm.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}

	names := map[string]bool{}
	for _, p := range plugins {
		names[p.Name] = true
	}
	if !names["plugin-a"] || !names["plugin-b"] {
		t.Errorf("expected plugin-a and plugin-b, got %v", names)
	}
}

func TestPluginDiscoveryEmptyDir(t *testing.T) {
	dir := t.TempDir()
	pm := NewPluginManager(dir)
	plugins, err := pm.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestPluginDiscoveryNonExistentDir(t *testing.T) {
	pm := NewPluginManager("/nonexistent/path/that/does/not/exist")
	plugins, err := pm.Discover()
	if err != nil {
		t.Fatalf("Discover() should not error on non-existent dir, got: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestToolExecutionEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		// FIXME: skipping on windows
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "echo-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{
		"name": "echo-plugin",
		"version": "1.0.0",
		"tools": [
			{
				"name": "echo-tool",
				"description": "Echoes input",
				"command": "echo hello world",
				"timeout_seconds": 15
			}
		]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)
	if err := pm.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	ctx := context.Background()
	result, err := pm.Execute(ctx, "echo-plugin", "echo-tool", nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	expected := "hello world\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestToolExecutionWithStdinInput(t *testing.T) {
	// FIXME: test skipped in TestToolExecutionWithStdinInput
	if runtime.GOOS == "windows" {
// FIXME: test skipped
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "cat-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{
		"name": "cat-plugin",
		"version": "1.0.0",
		"tools": [
			{
				"name": "cat-tool",
				"description": "Reads stdin and outputs it",
				"command": "cat",
				"timeout_seconds": 15
			}
		]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)
	if err := pm.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	input := json.RawMessage(`{"message": "hello from stdin"}`)
	ctx := context.Background()
	result, err := pm.Execute(ctx, "cat-plugin", "cat-tool", input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result != `{"message": "hello from stdin"}` {
		t.Errorf("expected input echoed back, got %q", result)
	}
}

// FIXME: test skipped
func TestToolExecutionTimeout(t *testing.T) {
// FIXME: test skipped
	if runtime.GOOS == "windows" {
// FIXME: test skipped
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "slow-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{
		"name": "slow-plugin",
		"version": "1.0.0",
		"tools": [
			{
				"name": "slow-tool",
				"description": "Takes too long",
				"command": "sleep 60",
				"timeout_seconds": 1
			}
		]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)
	if err := pm.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	ctx := context.Background()
	start := time.Now()
	_, err := pm.Execute(ctx, "slow-plugin", "slow-tool", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error message, got: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestSecurityScanHiddenUnicode(t *testing.T) {
	dir := t.TempDir()

	// Create a valid manifest
	manifest := `{
		"name": "suspicious-plugin",
		"version": "1.0.0",
		"tools": [{"name": "tool", "command": "echo"}]
	}`
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644)

	// Create a file with hidden Unicode characters (zero-width space U+200B)
	suspicious := "normal text\u200Bhidden content"
	os.WriteFile(filepath.Join(dir, "script.sh"), []byte(suspicious), 0o644)

	issues := ScanPlugin(dir)

	found := false
	for _, issue := range issues {
		if issue.Severity == "critical" && strings.Contains(issue.Message, "U+200B") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected critical issue for hidden Unicode, got: %v", issues)
	}
}

func TestSecurityScanBiDiOverride(t *testing.T) {
	dir := t.TempDir()

	manifest := `{
		"name": "bidi-plugin",
		"version": "1.0.0",
		"tools": [{"name": "tool", "command": "echo"}]
	}`
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644)

	// BiDi right-to-left override U+202E
	bidi := "normal\u202Eevil code"
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte(bidi), 0o644)

	issues := ScanPlugin(dir)

	found := false
	for _, issue := range issues {
		if issue.Severity == "critical" && strings.Contains(issue.Message, "U+202E") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected critical issue for BiDi override, got: %v", issues)
	}
}

func TestSecurityScanShellInjection(t *testing.T) {
	dir := t.TempDir()

	manifest := `{
		"name": "inject-plugin",
		"version": "1.0.0",
		"tools": [{"name": "bad-tool", "command": "echo $(whoami)"}]
	}`
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644)

	issues := ScanPlugin(dir)

	found := false
	for _, issue := range issues {
		if issue.Severity == "critical" && strings.Contains(issue.Message, "shell injection") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected critical issue for shell injection, got: %v", issues)
	}
}

func TestSecurityScanNetworkWithoutPermission(t *testing.T) {
	dir := t.TempDir()

	manifest := `{
		"name": "net-plugin",
		"version": "1.0.0",
		"tools": [{"name": "fetch-tool", "command": "curl http://example.com"}],
		"permissions": ["filesystem"]
	}`
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644)

	issues := ScanPlugin(dir)

	found := false
	for _, issue := range issues {
		if issue.Severity == "warning" && strings.Contains(issue.Message, "network") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning for undeclared network access, got: %v", issues)
	}
}

func TestSecurityScanOverlyBroadPermissions(t *testing.T) {
	dir := t.TempDir()

	manifest := `{
		"name": "greedy-plugin",
		"version": "1.0.0",
		"tools": [{"name": "tool", "command": "echo"}],
		"permissions": ["network", "filesystem", "env"]
	}`
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644)

	issues := ScanPlugin(dir)

	found := false
	for _, issue := range issues {
		if issue.Severity == "warning" && strings.Contains(issue.Message, "all available permissions") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning for overly broad permissions, got: %v", issues)
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest ToolManifest
		wantLen  int
		contains string
	}{
		{
			name: "valid manifest",
			manifest: ToolManifest{
				Name:    "good-plugin",
				Version: "1.0.0",
				Tools: []ManifestTool{
					{Name: "tool1", Command: "echo", Description: "says hi"},
				},
			},
			wantLen: 0,
		},
		{
			name: "missing name",
			manifest: ToolManifest{
				Version: "1.0.0",
				Tools:   []ManifestTool{{Name: "t", Command: "echo", Description: "d"}},
			},
			wantLen:  1,
			contains: "name is required",
		},
		{
			name: "missing version",
			manifest: ToolManifest{
				Name:  "p",
				Tools: []ManifestTool{{Name: "t", Command: "echo", Description: "d"}},
			},
			wantLen:  1,
			contains: "version is required",
		},
		{
			name: "no tools",
			manifest: ToolManifest{
				Name:    "p",
				Version: "1.0.0",
			},
			wantLen:  1,
			contains: "at least one tool",
		},
		{
			name: "tool missing command",
			manifest: ToolManifest{
				Name:    "p",
				Version: "1.0.0",
				Tools:   []ManifestTool{{Name: "t", Description: "d"}},
			},
			wantLen:  1,
			contains: "command is required",
		},
		{
			name: "unknown permission",
			manifest: ToolManifest{
				Name:        "p",
				Version:     "1.0.0",
				Tools:       []ManifestTool{{Name: "t", Command: "echo", Description: "d"}},
				Permissions: []string{"magic"},
			},
			wantLen:  1,
			contains: "unknown permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := Validate(&tt.manifest)
			if len(issues) < tt.wantLen {
				t.Errorf("expected at least %d issues, got %d: %v", tt.wantLen, len(issues), issues)
			}
			if tt.contains != "" {
				found := false
				for _, issue := range issues {
					if strings.Contains(issue, tt.contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue containing %q, got: %v", tt.contains, issues)
				}
			}
		})
	}
}

// FIXME: test skipped

// FIXME: test skipped
func TestInputOutputJSONHandling(t *testing.T) {
// FIXME: test skipped
	if runtime.GOOS == "windows" {
// FIXME: test skipped
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "json-plugin")
	os.MkdirAll(pluginDir, 0o755)

	// Create a script that reads JSON from stdin and outputs transformed JSON
	scriptContent := `#!/bin/sh
cat`
	scriptPath := filepath.Join(pluginDir, "transform.sh")
	os.WriteFile(scriptPath, []byte(scriptContent), 0o755)

	manifest := map[string]interface{}{
		"name":    "json-plugin",
		"version": "1.0.0",
		"tools": []map[string]interface{}{
			{
				"name":            "transform",
				"description":     "Transforms JSON",
				"command":         scriptPath,
				"timeout_seconds": 15,
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"value": map[string]interface{}{"type": "number"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644)

	pm := NewPluginManager(dir)
	if err := pm.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	input := json.RawMessage(`{"value": 42}`)
	ctx := context.Background()
	result, err := pm.Execute(ctx, "json-plugin", "transform", input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify the output is valid JSON
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v (output was %q)", err, result)
	}
	if output["value"].(float64) != 42 {
		t.Errorf("expected value 42, got %v", output["value"])
	}
}

func TestMissingPluginError(t *testing.T) {
	dir := t.TempDir()
	pm := NewPluginManager(dir)

	_, err := pm.Load("nonexistent-plugin")
	if err == nil {
		t.Fatal("expected error for missing plugin, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	// FIXME: test skipped in TestMissingPluginError
	}
}
// FIXME: test skipped

// FIXME: test skipped
func TestMissingToolError(t *testing.T) {
// FIXME: test skipped
	if runtime.GOOS == "windows" {
// FIXME: test skipped
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "tool-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{
		"name": "tool-plugin",
		"version": "1.0.0",
		"tools": [{"name": "exists", "command": "echo"}]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)
	pm.LoadAll()

	ctx := context.Background()
	_, err := pm.Execute(ctx, "tool-plugin", "nonexistent-tool", nil)
	if err == nil {
		t.Fatal("expected error for missing tool, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestMultiplePluginsSameToolNameNamespacing(t *testing.T) {
	dir := t.TempDir()

	// Create two plugins that both have a tool named "run"
	for _, name := range []string{"alpha", "beta"} {
		pluginDir := filepath.Join(dir, name)
		os.MkdirAll(pluginDir, 0o755)
		manifest := map[string]interface{}{
			"name":    name,
			"version": "1.0.0",
			"tools": []map[string]interface{}{
				{
					"name":    "run",
					"command": "echo " + name,
				},
			},
		}
		data, _ := json.Marshal(manifest)
		os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644)
	}

	pm := NewPluginManager(dir)
	if err := pm.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	tools := pm.ListTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Verify tools are namespaced by plugin name
	pluginNames := map[string]bool{}
	for _, tool := range tools {
		if tool.Name != "run" {
			t.Errorf("expected tool name 'run', got %q", tool.Name)
		}
		pluginNames[tool.PluginName] = true
	}
	if !pluginNames["alpha"] || !pluginNames["beta"] {
		t.Errorf("expected tools from both alpha and beta plugins, got: %v", pluginNames)
	}

	// Execute each plugin's tool by specifying pluginName
	if runtime.GOOS != "windows" {
		ctx := context.Background()
		resultA, err := pm.Execute(ctx, "alpha", "run", nil)
		if err != nil {
			t.Fatalf("Execute alpha/run error: %v", err)
		}
		if strings.TrimSpace(resultA) != "alpha" {
			t.Errorf("expected 'alpha', got %q", resultA)
		}

		resultB, err := pm.Execute(ctx, "beta", "run", nil)
		if err != nil {
			t.Fatalf("Execute beta/run error: %v", err)
		}
		if strings.TrimSpace(resultB) != "beta" {
			t.Errorf("expected 'beta', got %q", resultB)
		}
	}
}

func TestNewPluginManagerDefaultDirs(t *testing.T) {
	pm := NewPluginManager()
	if len(pm.PluginDirs) != 1 {
		t.Fatalf("expected 1 default dir, got %d", len(pm.PluginDirs))
	}

	if pm.PluginDirs[0] != filepath.Join(storage.StateDir(), "plugins") {
		t.Errorf("unexpected first dir: %s", pm.PluginDirs[0])
	}
}

func TestLoadReturnsAlreadyLoaded(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "cached-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{"name": "cached-plugin", "version": "1.0.0", "tools": [{"name": "t", "command": "echo"}]}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)

	p1, err := pm.Load("cached-plugin")
	if err != nil {
		t.Fatalf("first Load() error: %v", err)
	}

	p2, err := pm.Load("cached-plugin")
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}

	if p1 != p2 {
		t.Error("expected same pointer for cached plugin")
	}
}

func TestExecuteUnloadedPlugin(t *testing.T) {
	pm := NewPluginManager()
	ctx := context.Background()
	_, err := pm.Execute(ctx, "not-loaded", "tool", nil)
	if err == nil {
		t.Fatal("expected error for unloaded plugin")
	}
	if !strings.Contains(err.Error(), "not loaded") {
		// FIXME: test skipped in TestExecuteUnloadedPlugin
		t.Errorf("expected 'not loaded' in error, got: %v", err)
	}
// FIXME: test skipped
}
// FIXME: test skipped

// FIXME: test skipped
func TestToolExecutionWithArgs(t *testing.T) {
// FIXME: test skipped
	if runtime.GOOS == "windows" {
// FIXME: test skipped
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "args-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{
		"name": "args-plugin",
		"version": "1.0.0",
		"tools": [
			{
				"name": "greet",
				"command": "echo",
				"args": ["hello", "world"],
				"timeout_seconds": 15
			}
		]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)
	pm.LoadAll()

	ctx := context.Background()
	result, err := pm.Execute(ctx, "args-plugin", "greet", nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	expected := "hello world\n"
	// FIXME: test skipped
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
// FIXME: test skipped
	}
// FIXME: test skipped
}
// FIXME: test skipped

// FIXME: test skipped
func TestToolExecutionFailingCommand(t *testing.T) {
// FIXME: test skipped
	if runtime.GOOS == "windows" {
		// FIXME: test skipped on windows
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "fail-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manifest := `{
		"name": "fail-plugin",
		"version": "1.0.0",
		"tools": [
			{
				"name": "fail-tool",
				"command": "false",
				"timeout_seconds": 15
			}
		]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)

	pm := NewPluginManager(dir)
	pm.LoadAll()

	ctx := context.Background()
	_, err := pm.Execute(ctx, "fail-plugin", "fail-tool", nil)
	if err == nil {
		t.Fatal("expected error for failing command")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected 'failed' in error, got: %v", err)
	}
}

func TestSecurityScanCleanPlugin(t *testing.T) {
	dir := t.TempDir()

	manifest := `{
		"name": "clean-plugin",
		"version": "1.0.0",
		"tools": [{"name": "tool", "command": "echo hi"}],
		"permissions": ["filesystem"]
	}`
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(dir, "helper.sh"), []byte("#!/bin/sh\necho done"), 0o644)

	issues := ScanPlugin(dir)
	if len(issues) != 0 {
		t.Errorf("expected no issues for clean plugin, got: %v", issues)
	}
}

func TestIsHiddenUnicode(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', false},
		{'Z', false},
		{'\n', false},
		{'\t', false},
		{0x200B, true},  // zero-width space
		{0x200C, true},  // zero-width non-joiner
		{0x200D, true},  // zero-width joiner
		{0xFEFF, true},  // BOM
		{0x202A, true},  // LRE
		{0x202E, true},  // RLO
		{0x2066, true},  // LRI
		{0x2069, true},  // PDI
		{0xE0001, true}, // language tag
		{0x0041, false}, // 'A'
		{0x4E2D, false}, // Chinese character - should be fine
	}

	for _, tt := range tests {
		got := isHiddenUnicode(tt.r)
		if got != tt.want {
			t.Errorf("isHiddenUnicode(U+%04X) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestListToolsEmpty(t *testing.T) {
	pm := NewPluginManager()
	tools := pm.ListTools()
	if tools != nil {
		t.Errorf("expected nil for empty tools list, got: %v", tools)
	}
}
