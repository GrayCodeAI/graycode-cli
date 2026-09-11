package catalogtest

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

//go:embed testdata/minimal_v1.json
var minimalCatalogJSON []byte

var (
	globalOnce sync.Once
	globalPath string
)

// InstallGlobal writes the test catalog to a temp file and sets EYRIE_MODEL_CATALOG_PATH.
// Call from TestMain; returns cleanup to unset env.
func InstallGlobal() (cleanup func()) {
	globalOnce.Do(func() {
		dir, err := os.MkdirTemp("", "hawk-catalog-*")
		if err != nil {
			// In TestMain, we can't use t.Fatal, so log and exit.
			// This is a test helper, so panicking is acceptable but we'll use a clearer message.
			panic("catalogtest: failed to create temp dir: " + err.Error())
		}
		globalPath = filepath.Join(dir, "model_catalog.json")
		if err := os.WriteFile(globalPath, minimalCatalogJSON, 0o600); err != nil {
			panic("catalogtest: failed to write catalog: " + err.Error())
		}
		_ = os.Setenv("EYRIE_MODEL_CATALOG_PATH", globalPath)
	})
	return func() {
		_ = os.Unsetenv("EYRIE_MODEL_CATALOG_PATH")
	}
}

// Install sets EYRIE_MODEL_CATALOG_PATH for a single test (per-test temp file).
func Install(t testing.TB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model_catalog.json")
	if err := os.WriteFile(path, minimalCatalogJSON, 0o600); err != nil {
		t.Fatalf("catalogtest: failed to write catalog: %v", err)
	}
	t.Setenv("EYRIE_MODEL_CATALOG_PATH", path)
}
