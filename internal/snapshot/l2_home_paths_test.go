package snapshot

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestL2DefaultPathsAreHomeRelative is a regression guard for L2 — when the
// state-store constructors are called with empty/zero args, their default
// paths must be absolute and live under the user's home dir
// (~/.graycode/...), not relative to <cwd>. Pre-fix, the defaults were strings
// like ".graycode/snapshots" and ".graycode/experience" which leaked into
// <cwd>/cmd/.graycode/ when graycode was run from its own source tree.
func TestL2DefaultPathsAreHomeRelative(t *testing.T) {
	// Make the regression deterministic under CI and sandboxed runners. The
	// production default is the configured state root, which may intentionally
	// differ from HOME via GRAYCODE_STATE_DIR.
	stateRoot := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", stateRoot)
	wantPrefix := filepath.Clean(stateRoot) + string(filepath.Separator)

	check := func(name, got string) {
		t.Helper()
		if !filepath.IsAbs(got) {
			t.Errorf("%s: default path %q is not absolute", name, got)
			return
		}
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("%s: default path %q does not start with state root %q", name, got, stateRoot)
		}
	}

	// NewSnapshotStore("") default
	ss := NewSnapshotStore("")
	check("NewSnapshotStore", ss.Dir)

	// New(<projectDir>) default — shadowDir is now home-relative, not
	// relative to projectDir.
	tracker := New(t.TempDir())
	check("New(tracker)", tracker.shadowDir)
}
