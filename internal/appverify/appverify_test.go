package appverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDetectNextJS(t *testing.T) {
	root := writeProject(t, map[string]string{
		"package.json": `{"name":"app","scripts":{"dev":"next dev","build":"next build","test":"jest"},
			"dependencies":{"next":"14.0.0","react":"18.0.0"}}`,
	})
	r := Detect(root)
	if r.Ecosystem != "node" || r.AppKind != "web" || r.Port != 3000 {
		t.Fatalf("recipe = %+v", r)
	}
	if r.SmokeKind != SmokeHTTP || r.SmokeTarget() != "http://127.0.0.1:3000/" {
		t.Fatalf("smoke = %q target %q", r.SmokeKind, r.SmokeTarget())
	}
	want := []string{"npm", "run", "dev"}
	if strings.Join(r.Start, " ") != strings.Join(want, " ") {
		t.Fatalf("start = %v", r.Start)
	}
}

func TestDetectNodeLibraryNoSmoke(t *testing.T) {
	root := writeProject(t, map[string]string{
		"package.json": `{"name":"lib","scripts":{"test":"vitest"},"devDependencies":{"vite":"5"}}`,
	})
	r := Detect(root)
	// vite is a web framework marker; a lib with only vite and no start script
	// still has no boot path.
	if r.Start != nil {
		t.Fatalf("unexpected start %v", r.Start)
	}
	if r.SmokeKind == SmokeHTTP {
		t.Fatalf("library must not claim http smoke: %+v", r)
	}
}

func TestDetectGoCLIAndLibrary(t *testing.T) {
	cliRoot := writeProject(t, map[string]string{"go.mod": "module x\n\ngo 1.22\n", "main.go": "package main\n"})
	r := Detect(cliRoot)
	if r.Ecosystem != "go" || r.AppKind != "cli" || r.SmokeKind != SmokeCLI {
		t.Fatalf("cli recipe = %+v", r)
	}
	libRoot := writeProject(t, map[string]string{"go.mod": "module y\n\ngo 1.22\n"})
	r = Detect(libRoot)
	if r.AppKind != "library" || r.SmokeKind != SmokeNone {
		t.Fatalf("lib recipe = %+v", r)
	}
}

func TestDetectDjango(t *testing.T) {
	root := writeProject(t, map[string]string{
		"manage.py":        "#!/usr/bin/env python\n",
		"requirements.txt": "django\n",
	})
	r := Detect(root)
	if r.Ecosystem != "python" || r.AppLabel != "django app" || r.Port != 8000 || r.SmokeKind != SmokeHTTP {
		t.Fatalf("recipe = %+v", r)
	}
}

func TestDetectRust(t *testing.T) {
	root := writeProject(t, map[string]string{"Cargo.toml": "[package]\nname=\"x\"\n"})
	r := Detect(root)
	if r.Ecosystem != "rust" || len(r.Test) == 0 || r.SmokeKind != SmokeCLI {
		t.Fatalf("recipe = %+v", r)
	}
}

func TestDetectUnknown(t *testing.T) {
	r := Detect(t.TempDir())
	if r.Ecosystem != "unknown" || r.SmokeKind != SmokeNone || len(r.Notes) == 0 {
		t.Fatalf("recipe = %+v", r)
	}
}

func TestManifestRoundTripAndPriority(t *testing.T) {
	root := t.TempDir()
	r, existed, err := LoadOrDetect(root)
	if err != nil || existed {
		t.Fatalf("LoadOrDetect = (%+v,%v,%v)", r, existed, err)
	}
	if _, err := os.Stat(ManifestPath(root)); err != nil {
		t.Fatalf("manifest not persisted: %v", err)
	}
	// Second load reads the manifest, not detection.
	again, existed2, err := LoadOrDetect(root)
	if err != nil || !existed2 {
		t.Fatalf("second LoadOrDetect = (%+v,%v,%v)", again, existed2, err)
	}
	if again.Ecosystem != r.Ecosystem {
		t.Fatalf("round trip mismatch")
	}
}

func TestLoadManifestCorruptIsError(t *testing.T) {
	root := writeProject(t, map[string]string{
		".graycode/verify/environment.json": "{not json",
	})
	if _, err := LoadManifest(root); err == nil {
		t.Fatal("expected error for corrupt manifest")
	}
}

func TestNormalizeFiltersGarbage(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{
		"ecosystem": " Node! ",
		"appKind":   "WEB",
		"port":      99999,
		"smokeKind": "bogus",
		"install":   []string{"npm", "", "ci\ninjected", "ci"},
		"start":     []string{"npm", "start"},
	})
	r, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if r.Ecosystem != "node" || r.AppKind != "web" {
		t.Fatalf("tokens not sanitized: %+v", r)
	}
	if r.Port != 0 {
		t.Fatalf("out-of-range port kept: %d", r.Port)
	}
	if r.SmokeKind != SmokeNone {
		t.Fatalf("bogus smoke kind not reset: %q", r.SmokeKind)
	}
	for _, a := range r.Install {
		if strings.ContainsAny(a, "\n\r\x00") {
			t.Fatalf("unsafe arg survived: %q", a)
		}
	}
	if got := strings.Join(r.Install, " "); got != "npm ci" {
		t.Fatalf("install = %q", got)
	}
}

func TestNormalizeHTTPRequiresStart(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{"ecosystem": "go", "smokeKind": "http", "port": 8080})
	r, err := Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.SmokeKind != SmokeNone {
		t.Fatalf("http without start must downgrade to none, got %q", r.SmokeKind)
	}
}

func TestBuildVerifyPromptPhases(t *testing.T) {
	r := Recipe{
		Ecosystem: "node", AppKind: "web", AppLabel: "next.js app",
		Install:   []string{"npm", "ci"},
		Build:     []string{"npm", "run", "build"},
		Test:      []string{"npm", "test"},
		Start:     []string{"npm", "run", "dev"},
		Port:      3000,
		SmokeKind: SmokeHTTP,
	}
	p := BuildVerifyPrompt(r)
	for _, want := range []string{
		"Phase 1 — Setup", "Phase 2 — Build and test", "Phase 3 — Boot the app",
		"Phase 4 — Evidence", "Phase 5 — Teardown",
		"http://127.0.0.1:3000/", EvidenceDir, "NOT success",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}
