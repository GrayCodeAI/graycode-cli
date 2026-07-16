package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeAdditiveOnly(t *testing.T) {
	user := TOMLConfig{
		Profile: ProfileStrict,
		Profiles: map[string]ProfileConfig{
			"mine": {Mode: "workspace"},
		},
		DenyGlobs: []string{"**/.env"},
	}
	project := TOMLConfig{
		Profile: ProfileDevbox, // weaker than strict — should error
		Profiles: map[string]ProfileConfig{
			"mine":   {Mode: "off"}, // redefinition ignored
			"projex": {Mode: "workspace"},
		},
		DenyGlobs: []string{"**/secrets/**"},
	}
	_, err := MergeConfigs(user, project)
	if err == nil {
		t.Fatal("expected error when project weakens profile")
	}

	project.Profile = ProfileStrict
	merged, err := MergeConfigs(user, project)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := merged.Profiles["projex"]; !ok {
		t.Fatal("expected additive project profile")
	}
	// mine should remain user's version (workspace), not off
	if merged.Profiles["mine"].Mode != "workspace" {
		t.Fatalf("mine mode=%q", merged.Profiles["mine"].Mode)
	}
	if len(merged.DenyGlobs) != 2 {
		t.Fatalf("deny globs=%v", merged.DenyGlobs)
	}
}

func TestEffectiveFromAndDeny(t *testing.T) {
	cfg := TOMLConfig{
		Profile:   ProfileReadOnly,
		Profiles:  builtinProfiles(),
		DenyGlobs: []string{"**/.env", "secrets.txt"},
	}
	eff, err := EffectiveFrom(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Mode != ModeStrict {
		t.Fatalf("mode=%s", eff.Mode)
	}
	if !eff.PathDenied("/proj/.env") {
		t.Fatal("expected .env denied")
	}
	if !eff.PathDenied("/proj/secrets.txt") {
		t.Fatal("expected secrets.txt denied")
	}
	if eff.PathDenied("/proj/main.go") {
		t.Fatal("main.go should not be denied")
	}
}

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sandbox.toml")
	content := `
profile = "workspace"
deny_globs = ["**/.env"]

[profiles.extra]
mode = "strict"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTOML(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "workspace" || len(cfg.DenyGlobs) != 1 {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Profiles["extra"].Mode != "strict" {
		t.Fatalf("extra=%+v", cfg.Profiles["extra"])
	}
}
