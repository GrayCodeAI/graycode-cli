package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Exists(f) {
		t.Errorf("Exists(%q) = false, want true", f)
	}
	if Exists(filepath.Join(dir, "missing.txt")) {
		t.Error("Exists(missing) = true, want false")
	}
}
