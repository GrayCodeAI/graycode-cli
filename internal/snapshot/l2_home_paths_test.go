package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestL2DefaultPathsAreHomeRelative is a regression guard for L2 — when the
// state-store constructors are called with empty/zero args, their default
// paths must be absolute and live under the user's home dir
// (~/.hawk/...), not relative to <cwd>. Pre-fix, the defaults were strings
// like ".hawk/snapshots" and ".hawk/experience" which leaked into
// <cwd>/cmd/.hawk/ when hawk was run from its own source tree.
func TestL2DefaultPathsAreHomeRelative(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	if home == "" {
		t.Fatal("os.UserHomeDir returned empty string")
	}

	// Sanitize HOME so we can compare reliably (filepath.Clean strips
	// trailing separators).
	wantPrefix := filepath.Clean(home) + string(filepath.Separator)

	check := func(name, got string) {
		t.Helper()
		if !filepath.IsAbs(got) {
			t.Errorf("%s: default path %q is not absolute", name, got)
			return
		}
		// On macOS temp dirs may live under /private/var/... while HOME
		// resolves to /var/...; compare both forms.
		if !strings.HasPrefix(got, wantPrefix) && !strings.HasPrefix(got, filepath.Clean(home)) {
			t.Errorf("%s: default path %q does not start with home dir %q", name, got, home)
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
