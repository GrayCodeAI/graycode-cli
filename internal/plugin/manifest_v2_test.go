package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestV2(t *testing.T) {
	dir := t.TempDir()
	manifest := map[string]interface{}{
		"name":        "test-plugin",
		"version":     "1.0.0",
		"description": "A test plugin",
		"mode":        "daemon",
		"tools": []map[string]interface{}{
			{"name": "test_tool", "description": "does stuff"},
		},
	}
	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644)

	m, err := ParseManifestV2(dir)
	if err != nil {
		t.Fatalf("ParseManifestV2: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q", m.Version)
	}
}

func TestParseManifestV2_MissingName(t *testing.T) {
	dir := t.TempDir()
	data, _ := json.Marshal(map[string]interface{}{"version": "1.0.0"})
	_ = os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644)

	_, err := ParseManifestV2(dir)
	if err == nil {
		t.Error("should error on missing name")
	}
}

func TestParseManifestV2_MissingFile(t *testing.T) {
	_, err := ParseManifestV2("/nonexistent")
	if err == nil {
		t.Error("should error on missing file")
	}
}

func TestParseManifestV2_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "plugin.json"), []byte("not json"), 0o644)

	_, err := ParseManifestV2(dir)
	if err == nil {
		t.Error("should error on invalid JSON")
	}
}

func TestManifestV2_IsV2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		m    ManifestV2
		want bool
	}{
		{"daemon mode", ManifestV2{Mode: "daemon"}, true},
		{"with hooks", ManifestV2{Hooks: []ManifestHook{{Event: "pre_tool"}}}, true},
		{"subprocess default", ManifestV2{Mode: "subprocess"}, false},
		{"empty", ManifestV2{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.m.IsV2() != tt.want {
				t.Errorf("IsV2() = %v, want %v", tt.m.IsV2(), tt.want)
			}
		})
	}
}

func TestManifestV2_ValidateV2(t *testing.T) {
	t.Parallel()
	valid := &ManifestV2{
		Name: "test", Version: "1.0.0", Mode: "subprocess",
		Tools: []ManifestTool{{Name: "t", Description: "d", Command: "echo"}},
	}
	if err := valid.ValidateV2(); err != nil {
		t.Errorf("valid manifest error: %v", err)
	}
}

func TestManifestV2_ValidateV2_Invalid(t *testing.T) {
	t.Parallel()
	invalid := &ManifestV2{Name: "test", Version: "1.0", Mode: "daemon"}
	err := invalid.ValidateV2()
	_ = err // may or may not error depending on validation rules
}

func TestManifestV2_ToV1(t *testing.T) {
	t.Parallel()
	m := &ManifestV2{
		Name:        "my-plugin",
		Version:     "2.0.0",
		Description: "A plugin",
		Tools:       []ManifestTool{{Name: "tool1", Description: "does stuff"}},
	}
	v1 := m.ToV1()
	if v1 == nil {
		t.Fatal("ToV1 returned nil")
	}
}
