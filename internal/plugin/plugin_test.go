package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		wantErr  bool
	}{
		{
			name: "valid",
			manifest: Manifest{
				Name:    "test-plugin",
				Version: "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: Manifest{
				Version: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "missing version",
			manifest: Manifest{
				Name: "test-plugin",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	manifestData := `{"name": "test", "version": "1.0.0", "description": "Test plugin"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifestData), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "test" {
		t.Fatalf("expected name 'test', got %q", m.Name)
	}
}

func TestInstallAndUninstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	srcDir := t.TempDir()
	manifestData := `{"name": "test-plugin", "version": "1.0.0"}`
	if err := os.WriteFile(filepath.Join(srcDir, "plugin.json"), []byte(manifestData), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(srcDir); err != nil {
		t.Fatal(err)
	}

	plugins, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	if uninstallErr := Uninstall("test-plugin"); uninstallErr != nil {
		t.Fatal(uninstallErr)
	}

	plugins, err = List()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestInstallRejectsCriticalSecurityIssue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	srcDir := t.TempDir()
	manifestData := `{"name":"bad-plugin","version":"1.0.0","tools":[{"name":"bad","description":"bad","command":"echo $(whoami)"}]}`
	if err := os.WriteFile(filepath.Join(srcDir, "plugin.json"), []byte(manifestData), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Install(srcDir)
	if err == nil {
		t.Fatal("Install() should reject plugin with critical scan issue")
	}
	if !strings.Contains(err.Error(), "plugin security scan failed") {
		t.Fatalf("Install() error = %v, want security scan failure", err)
	}
}

func TestPluginHookEnvKeySanitizesAndPrefixes(t *testing.T) {
	if got := pluginHookEnvKey("path=evil"); got != "HAWK_PATH_EVIL" {
		t.Fatalf("pluginHookEnvKey(path=evil) = %q", got)
	}
	if got := pluginHookEnvKey("tool name"); got != "HAWK_TOOL_NAME" {
		t.Fatalf("pluginHookEnvKey(tool name) = %q", got)
	}
	if got := pluginHookEnvKey(""); got != "HAWK_DATA" {
		t.Fatalf("pluginHookEnvKey(empty) = %q", got)
	}
}
