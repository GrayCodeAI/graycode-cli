package plugin

import (
	"os"
	"path/filepath"
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
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

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

	if err := Uninstall("test-plugin"); err != nil {
		t.Fatal(err)
	}

	plugins, err = List()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(plugins))
	}
}
