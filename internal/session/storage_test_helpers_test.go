package session

import (
	"path/filepath"
	"testing"
)

func setTestSessionsDir(t *testing.T, root string) string {
	t.Helper()
	stateDir := filepath.Join(root, "state")
	t.Setenv("HAWK_STATE_DIR", stateDir)
	return filepath.Join(stateDir, "sessions")
}
