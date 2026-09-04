package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// IsolateStorage configures isolated HOME and Graycode config/state/cache dirs for tests.
func IsolateStorage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	IsolateStorageIn(t, root)
	return root
}

// InstallHermeticStorage gives package-level TestMain functions writable,
// isolated storage without overriding explicit caller configuration. The
// returned cleanup restores the environment and removes the temporary root.
func InstallHermeticStorage() (func(), error) {
	root, err := os.MkdirTemp("", "graycode-test-storage-")
	if err != nil {
		return nil, err
	}
	keys := []string{"HOME", "GRAYCODE_CONFIG_DIR", "GRAYCODE_STATE_DIR", "GRAYCODE_CACHE_DIR", "GRAYCODE_ROUTER_CONFIG_DIR"}
	previous := make(map[string]string, len(keys))
	wasSet := make(map[string]bool, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		previous[key] = value
		wasSet[key] = ok
	}
	if err := os.Setenv("HOME", root); err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	for key, suffix := range map[string]string{
		"GRAYCODE_CONFIG_DIR":        "config",
		"GRAYCODE_STATE_DIR":         "state",
		"GRAYCODE_CACHE_DIR":         "cache",
		"GRAYCODE_ROUTER_CONFIG_DIR": "graycode-router-config",
	} {
		if _, ok := os.LookupEnv(key); !ok {
			if err := os.Setenv(key, filepath.Join(root, suffix)); err != nil {
				_ = os.RemoveAll(root)
				return nil, err
			}
		}
	}
	return func() {
		for _, key := range keys {
			if wasSet[key] {
				_ = os.Setenv(key, previous[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
		_ = os.RemoveAll(root)
	}, nil
}

// IsolateStorageIn configures isolated HOME and Graycode config/state/cache dirs rooted at root.
func IsolateStorageIn(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", root)
	storage.SetTestDirs(t, root)
}
