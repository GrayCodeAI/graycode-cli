package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCheckPathRejectsSiblingPrefix verifies that a sibling directory sharing
// the project root as a string prefix (e.g. /proj vs /proj-evil) is rejected.
func TestCheckPathRejectsSiblingPrefix(t *testing.T) {
	// Resolve symlinks in the temp dir (on macOS /var -> /private/var) so the
	// checker's symlink-resolution logic compares like with like.
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "proj")
	sibling := filepath.Join(parent, "proj-evil")
	for _, d := range []string{root, sibling} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	bc := NewBoundaryChecker(root)

	if v := bc.CheckPath(filepath.Join(sibling, "x.txt")); v == nil {
		t.Error("CheckPath accepted sibling-prefix path outside project root")
	}
	if bc.IsWithinProject(filepath.Join(sibling, "x.txt")) {
		t.Error("IsWithinProject accepted sibling-prefix path")
	}
	if v := bc.CheckPath(filepath.Join(root, "x.txt")); v != nil {
		t.Errorf("CheckPath rejected legitimate in-root path: %+v", v)
	}
	if !bc.IsWithinProject(root) {
		t.Error("IsWithinProject rejected the project root itself")
	}
}
