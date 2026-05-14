package tool

import "testing"

func TestLoadDescriptionExists(t *testing.T) {
	desc := LoadDescription("bash", "default fallback")
	expected := "Run a shell command and return stdout/stderr."
	if desc != expected {
		t.Errorf("LoadDescription(\"bash\") = %q, want %q", desc, expected)
	}
}

func TestLoadDescriptionFallback(t *testing.T) {
	fallback := "this is the fallback"
	desc := LoadDescription("nonexistent_tool_xyz", fallback)
	if desc != fallback {
		t.Errorf("LoadDescription(\"nonexistent_tool_xyz\") = %q, want fallback %q", desc, fallback)
	}
}
