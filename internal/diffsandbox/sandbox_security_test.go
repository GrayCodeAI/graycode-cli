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
