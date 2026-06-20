package testutil

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// IsolateStorage configures isolated HOME and Hawk config/state/cache dirs for tests.
func IsolateStorage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	IsolateStorageIn(t, root)
	return root
}

// IsolateStorageIn configures isolated HOME and Hawk config/state/cache dirs rooted at root.
func IsolateStorageIn(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", root)
	storage.SetTestDirs(t, root)
}
