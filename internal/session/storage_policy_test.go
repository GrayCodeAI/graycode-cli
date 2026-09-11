package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionPathDoesNotCreateProjectHawk(t *testing.T) {
	project := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("HAWK_STATE_DIR", state)

	path := SessionPath(project, "abc123")
	if strings.Contains(path, filepath.Join(project, ".hawk")) {
		t.Fatalf("SessionPath leaked project .hawk: %q", path)
	}
	if !strings.HasPrefix(path, state) {
		t.Fatalf("SessionPath = %q, want under %q", path, state)
	}
	if _, err := os.Stat(filepath.Join(project, ".hawk")); !os.IsNotExist(err) {
		t.Fatalf("SessionPath created project .hawk, stat err=%v", err)
	}
}
