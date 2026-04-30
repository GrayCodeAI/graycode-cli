package ide

import "testing"

func TestDefaultManifest(t *testing.T) {
	m := DefaultManifest()
	if m.Name != "hawk" {
		t.Fatalf("expected name 'hawk', got %q", m.Name)
	}
	if len(m.Contributes.Commands) == 0 {
		t.Fatal("expected commands")
	}
}

func TestGeneratePackageJSON(t *testing.T) {
	data, err := GeneratePackageJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}
}

func TestHints(t *testing.T) {
	hints := Hints()
	if len(hints) == 0 {
		t.Fatal("expected hints")
	}
}

func TestSuggestLSPConfig(t *testing.T) {
	config, err := SuggestLSPConfig("go")
	if err != nil {
		t.Fatal(err)
	}
	if config.Command != "gopls" {
		t.Fatalf("expected gopls, got %q", config.Command)
	}

	_, err = SuggestLSPConfig("unknown")
	if err == nil {
		t.Fatal("expected error for unknown language")
	}
}
