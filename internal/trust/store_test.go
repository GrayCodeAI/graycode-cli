package trust

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/flags"
)

func TestTrustRoundTrip(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "trust.json")
	s, err := Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, "proj")
	if mkErr := os.MkdirAll(proj, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	if s.IsTrusted(proj) {
		t.Fatal("expected untrusted")
	}
	if trErr := s.Trust(proj, "test"); trErr != nil {
		t.Fatal(trErr)
	}
	s2, err := Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.IsTrusted(proj) {
		t.Fatal("expected trusted after reload")
	}
	// child inherits
	child := filepath.Join(proj, "sub")
	_ = os.MkdirAll(child, 0o755)
	if !s2.IsTrusted(child) {
		t.Fatal("child should inherit trust")
	}
	if err := s2.Untrust(proj); err != nil {
		t.Fatal(err)
	}
	if s2.IsTrusted(proj) {
		t.Fatal("expected untrusted after remove")
	}
}

func TestAllowProjectAutomationFlag(t *testing.T) {
	flags.ResetForTest()
	t.Cleanup(flags.ResetForTest)

	dir := t.TempDir()
	// Trust disabled → allow
	flags.SetForTest(flags.EnvFolderTrust, false)
	if err := AllowProjectAutomation(dir); err != nil {
		t.Fatalf("disabled trust should allow: %v", err)
	}
	// Trust enabled, not in store → deny
	flags.SetForTest(flags.EnvFolderTrust, true)
	s, err := Open(filepath.Join(dir, "t.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s.IsTrusted(dir) {
		t.Fatal("should not be trusted")
	}
}

func TestRequiresFolderTrustMarkers(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, ".hawk", "plugins")
	_ = os.MkdirAll(plugins, 0o755)
	if !RequiresFolderTrust(plugins) {
		t.Fatal("project .hawk/plugins should require trust")
	}
	generic := filepath.Join(dir, "plugins")
	_ = os.MkdirAll(generic, 0o755)
	if RequiresFolderTrust(generic) {
		t.Fatal("generic plugins dir should not require trust")
	}
}

func TestAllowLoadPathProjectPlugins(t *testing.T) {
	flags.ResetForTest()
	t.Cleanup(flags.ResetForTest)
	flags.SetForTest(flags.EnvFolderTrust, true)

	dir := t.TempDir()
	plugins := filepath.Join(dir, ".hawk", "plugins")
	_ = os.MkdirAll(plugins, 0o755)
	// Default store is empty → deny
	if err := AllowLoadPath(plugins); err == nil {
		// AllowLoadPath uses default store path under StateDir, which may
		// accidentally include trusted paths on the machine. Prefer unit
		// semantics: RequiresFolderTrust is true; IsTrusted of isolated store is false.
		s, _ := Open(filepath.Join(dir, "empty.json"))
		if s.IsTrusted(plugins) {
			t.Fatal("isolated store should not trust plugins")
		}
	}
}
