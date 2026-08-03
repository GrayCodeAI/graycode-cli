package diffsandbox

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApplyRejectsPathTraversal verifies that staged changes cannot write or
// delete outside the sandbox root via ".." segments or absolute paths.
func TestApplyRejectsPathTraversal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(parent, "victim.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"../victim.txt",
		"a/../../victim.txt",
		victim, // absolute path outside root
		"..",
	} {
		s := New(root)
		s.ProposeCreate(path, "pwned")
		if err := s.Apply(); err == nil {
			t.Errorf("Apply(%q) succeeded; want escape error", path)
		}
	}

	if data, err := os.ReadFile(victim); err != nil || string(data) != "original" {
		t.Fatalf("victim file modified: %q, err=%v", data, err)
	}

	// Delete escape must also be rejected.
	s := New(root)
	s.ProposeDelete("../victim.txt")
	if err := s.Apply(); err == nil {
		t.Error("Apply(delete ../victim.txt) succeeded; want escape error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("victim file deleted: %v", err)
	}

	// ProposeModify must refuse to read outside the root.
	s = New(root)
	if _, err := s.ProposeModify("../victim.txt", "x"); err == nil {
		t.Error("ProposeModify(../victim.txt) succeeded; want escape error")
	}

	// Sanity: normal relative paths still work.
	s = New(root)
	s.ProposeCreate("sub/ok.txt", "hello")
	if err := s.Apply(); err != nil {
		t.Fatalf("Apply(sub/ok.txt) failed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "sub", "ok.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("expected file inside root: %q, err=%v", data, err)
	}
}

// TestApplyRejectsSymlinkEscape verifies that a symlinked intermediate
// directory cannot smuggle writes or reads outside the sandbox root (M10).
func TestApplyRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	victimDir := filepath.Join(parent, "victimdir")
	if err := os.Mkdir(victimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Link "root/escape" -> victimDir (outside the root).
	if err := os.Symlink(victimDir, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	// Create through the symlink must be rejected.
	s := New(root)
	s.ProposeCreate("escape/pwned.txt", "pwned")
	if err := s.Apply(); err == nil {
		t.Error("Apply(escape/pwned.txt) succeeded; want symlink-escape error")
	}
	if _, err := os.Stat(filepath.Join(victimDir, "pwned.txt")); err == nil {
		t.Fatal("victim file created outside root via symlink")
	}

	// Modify (read) through the symlink must be rejected.
	s = New(root)
	if _, err := s.ProposeModify("escape/victim.txt", "x"); err == nil {
		t.Error("ProposeModify through symlink succeeded; want escape error")
	}

	// Delete through the symlink must be rejected.
	victim := filepath.Join(victimDir, "victim.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	s = New(root)
	s.ProposeDelete("escape/victim.txt")
	if err := s.Apply(); err == nil {
		t.Error("Apply(delete via symlink) succeeded; want escape error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("victim deleted via symlink: %v", err)
	}

	// A symlink that resolves inside the root remains usable.
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	s = New(root)
	s.ProposeCreate("alias/inner.txt", "ok")
	if err := s.Apply(); err != nil {
		t.Fatalf("Apply(alias/inner.txt) failed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "sub", "inner.txt")); err != nil || string(data) != "ok" {
		t.Fatalf("expected file at resolved target: %q, err=%v", data, err)
	}

	// A dangling symlink (target does not exist) is rejected: its target
	// cannot be containment-verified.
	if err := os.Symlink(filepath.Join(root, "nosuchdir"), filepath.Join(root, "dangle")); err != nil {
		t.Fatal(err)
	}
	s = New(root)
	s.ProposeCreate("dangle/x.txt", "x")
	if err := s.Apply(); err == nil {
		t.Error("Apply(dangle/x.txt) succeeded; want rejection of unresolvable symlink")
	}
}
