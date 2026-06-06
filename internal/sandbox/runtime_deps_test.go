package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRuntimeConfig(t *testing.T) {
	dir := t.TempDir()
	hawkDir := filepath.Join(dir, ".hawk")
	if err := os.MkdirAll(hawkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  // install extra deps at build time
  "runtime_extra_deps": ["apt-get update", "pip install requests"],
  "runtime_startup_env_vars": {"FOO": "bar", "BAZ": "qux"}
}`
	if err := os.WriteFile(filepath.Join(hawkDir, "runtime.jsonc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadRuntimeConfig(dir)
	if cfg.IsEmpty() {
		t.Fatal("expected non-empty config")
	}
	if len(cfg.RuntimeExtraDeps) != 2 {
		t.Errorf("got %d extra deps, want 2", len(cfg.RuntimeExtraDeps))
	}
	if cfg.RuntimeStartupEnvVars["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", cfg.RuntimeStartupEnvVars["FOO"])
	}
}

func TestLoadRuntimeConfigMissing(t *testing.T) {
	cfg := LoadRuntimeConfig(t.TempDir())
	if !cfg.IsEmpty() {
		t.Errorf("missing file should yield empty config, got %+v", cfg)
	}
}

func TestLoadRuntimeConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	hawkDir := filepath.Join(dir, ".hawk")
	_ = os.MkdirAll(hawkDir, 0o755)
	_ = os.WriteFile(filepath.Join(hawkDir, "runtime.jsonc"), []byte("{ not json"), 0o644)
	cfg := LoadRuntimeConfig(dir)
	if !cfg.IsEmpty() {
		t.Errorf("malformed file should fail open to empty, got %+v", cfg)
	}
}

func TestExtraDepsDockerfileFragment(t *testing.T) {
	tests := []struct {
		name string
		deps []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"apt-get install -y curl"}, "RUN apt-get install -y curl\n"},
		{"multi", []string{"a", "b"}, "RUN a\nRUN b\n"},
		{"skips blanks", []string{"a", "  ", "b"}, "RUN a\nRUN b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := RuntimeConfig{RuntimeExtraDeps: tt.deps}
			if got := cfg.ExtraDepsDockerfileFragment(); got != tt.want {
				t.Errorf("fragment = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendExtraDeps(t *testing.T) {
	cfg := RuntimeConfig{RuntimeExtraDeps: []string{"pip install ruff"}}

	// Base without trailing newline gets one inserted.
	got := cfg.AppendExtraDeps("FROM python:3.12")
	want := "FROM python:3.12\nRUN pip install ruff\n"
	if got != want {
		t.Errorf("AppendExtraDeps = %q, want %q", got, want)
	}

	// Empty config returns input unchanged.
	empty := RuntimeConfig{}
	if out := empty.AppendExtraDeps("FROM scratch"); out != "FROM scratch" {
		t.Errorf("empty config should not modify dockerfile, got %q", out)
	}
}

func TestStartupEnvArgs(t *testing.T) {
	cfg := RuntimeConfig{RuntimeStartupEnvVars: map[string]string{"B": "2", "A": "1", "C": "3"}}
	got := cfg.StartupEnvArgs()
	// Sorted by key for determinism.
	want := []string{"-e", "A=1", "-e", "B=2", "-e", "C=3"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("StartupEnvArgs = %v, want %v", got, want)
	}

	var empty RuntimeConfig
	if empty.StartupEnvArgs() != nil {
		t.Error("empty config should return nil args")
	}
}

// TestDevEnvBuildComposesExtraDeps asserts the extra-deps RUN layers are
// composed into the Dockerfile that the build receives (no docker required:
// we capture the buildFn input).
func TestDevEnvBuildComposesExtraDeps(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dfPath := filepath.Join(projDir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte("FROM alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewDevEnvManager(projDir)
	mgr.SetRuntimeConfig(RuntimeConfig{RuntimeExtraDeps: []string{"apk add --no-cache git"}})

	var builtContent string
	mgr.buildFn = func(ctx context.Context, dockerfile, tag string) error {
		data, err := os.ReadFile(dockerfile)
		if err != nil {
			return err
		}
		builtContent = string(data)
		return nil
	}

	if _, err := mgr.GetOrBuild(context.Background(), dfPath); err != nil {
		t.Fatalf("GetOrBuild: %v", err)
	}

	if !strings.Contains(builtContent, "FROM alpine:3.20") {
		t.Errorf("built dockerfile missing base image: %q", builtContent)
	}
	if !strings.Contains(builtContent, "RUN apk add --no-cache git") {
		t.Errorf("built dockerfile missing extra-dep RUN layer: %q", builtContent)
	}
}

// TestDevEnvBuildEmptyConfigUnchanged asserts default behavior when no extra
// deps are configured: the original Dockerfile path is built directly.
func TestDevEnvBuildEmptyConfigUnchanged(t *testing.T) {
	dir := t.TempDir()
	dfPath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte("FROM alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := NewDevEnvManager(dir)
	mgr.SetRuntimeConfig(RuntimeConfig{}) // explicitly empty

	var builtPath string
	mgr.buildFn = func(ctx context.Context, dockerfile, tag string) error {
		builtPath = dockerfile
		return nil
	}
	if _, err := mgr.GetOrBuild(context.Background(), dfPath); err != nil {
		t.Fatalf("GetOrBuild: %v", err)
	}
	if builtPath != dfPath {
		t.Errorf("empty config should build original path, got %q want %q", builtPath, dfPath)
	}
}

// TestContainerStartupEnvComposed verifies the container's runtime env args
// are produced from a loaded config (composition check without docker).
func TestContainerStartupEnvComposed(t *testing.T) {
	cs := NewContainerSandbox(t.TempDir())
	cs.SetRuntimeConfig(RuntimeConfig{RuntimeStartupEnvVars: map[string]string{"HAWK_ENV": "test"}})
	args := cs.runtime.StartupEnvArgs()
	if strings.Join(args, " ") != "-e HAWK_ENV=test" {
		t.Errorf("env args = %v, want [-e HAWK_ENV=test]", args)
	}
}
