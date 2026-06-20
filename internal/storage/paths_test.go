package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageDirsRespectOverrides(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "cfg")
	stateDir := filepath.Join(t.TempDir(), "state")
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv(envConfigDir, configDir)
	t.Setenv(envStateDir, stateDir)
	t.Setenv(envCacheDir, cacheDir)

	if got := ConfigDir(); got != configDir {
		t.Fatalf("ConfigDir() = %q, want override %q", got, configDir)
	}
	if got := StateDir(); got != stateDir {
		t.Fatalf("StateDir() = %q, want override %q", got, stateDir)
	}
	if got := CacheDir(); got != cacheDir {
		t.Fatalf("CacheDir() = %q, want override %q", got, cacheDir)
	}
}

func TestProjectIDIsStableAndSafe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my project")

	first := ProjectID(root)
	second := ProjectID(root)
	if first != second {
		t.Fatalf("ProjectID not stable: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "my-project-") {
		t.Fatalf("ProjectID() = %q, want sanitized base prefix", first)
	}
	if strings.ContainsAny(first, string(filepath.Separator)+" ") {
		t.Fatalf("ProjectID() = %q, contains unsafe path characters", first)
	}
}

func TestProjectStateAndCacheUseHashedProjectRoot(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	cache := filepath.Join(t.TempDir(), "cache")
	project := filepath.Join(t.TempDir(), "repo")
	t.Setenv(envStateDir, state)
	t.Setenv(envCacheDir, cache)

	id := ProjectID(project)
	if got := ProjectStateDir(project); got != filepath.Join(state, "projects", id) {
		t.Fatalf("ProjectStateDir() = %q", got)
	}
	if got := ProjectCacheDir(project); got != filepath.Join(cache, "projects", id) {
		t.Fatalf("ProjectCacheDir() = %q", got)
	}
}
