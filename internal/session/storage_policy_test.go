package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionPathDoesNotCreateProjectGraycode(t *testing.T) {
	project := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("GRAYCODE_STATE_DIR", state)

	path := SessionPath(project, "abc123")
	if strings.Contains(path, filepath.Join(project, ".graycode")) {
		t.Fatalf("SessionPath leaked project .graycode: %q", path)
	}
	if !strings.HasPrefix(path, state) {
		t.Fatalf("SessionPath = %q, want under %q", path, state)
	}
	if _, err := os.Stat(filepath.Join(project, ".graycode")); !os.IsNotExist(err) {
		t.Fatalf("SessionPath created project .graycode, stat err=%v", err)
	}
}
