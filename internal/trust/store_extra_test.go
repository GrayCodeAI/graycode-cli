package trust

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_List(t *testing.T) {
	s := &Store{
		Entries: map[string]Entry{
			"/path1": {Path: "/path1", TrustedAt: time.Now()},
			"/path2": {Path: "/path2", TrustedAt: time.Now()},
		},
	}

	list := s.List()
	if len(list) != 2 {
		t.Errorf("List() returned %d entries, want 2", len(list))
	}
}

func TestStore_List_Empty(t *testing.T) {
	s := &Store{Entries: map[string]Entry{}}
	list := s.List()
	if len(list) != 0 {
		t.Errorf("List() on empty store returned %d entries, want 0", len(list))
	}
}

func TestStore_List_NilEntries(t *testing.T) {
	s := &Store{}
	list := s.List()
	if len(list) != 0 {
		t.Errorf("List() with nil entries returned %d entries, want 0", len(list))
	}
}

func TestStore_List_ModificationSafety(t *testing.T) {
	s := &Store{
		Entries: map[string]Entry{
			"/path1": {Path: "/path1", TrustedAt: time.Now()},
		},
	}

	list := s.List()
	if len(list) > 0 {
		list[0].Path = "/modified"
	}

	// Original should be unchanged
	if s.Entries["/path1"].Path != "/path1" {
		t.Error("List() should return a copy, not a reference")
	}
}

func TestCanonicalize(t *testing.T) {
	path, err := canonicalize("/tmp")
	if err != nil {
		t.Fatalf("canonicalize error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestCanonicalize_EmptyPath(t *testing.T) {
	path, err := canonicalize("")
	if err != nil {
		t.Fatalf("canonicalize error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path for empty input")
	}
}

func TestCanonicalize_NonExistentPath(t *testing.T) {
	path, err := canonicalize("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("canonicalize error: %v", err)
	}
	if path != "/nonexistent/path/xyz" {
		t.Errorf("canonicalize = %q, want %q", path, "/nonexistent/path/xyz")
	}
}

func TestIsProjectPath_ProjectPath(t *testing.T) {
	result := IsProjectPath("/some/project/path")
	if !result {
		t.Error("expected /some/project/path to be a project path")
	}
}

func TestIsProjectPath_EmptyPath(t *testing.T) {
	result := IsProjectPath("")
	if !result {
		t.Error("expected empty path (cwd) to be a project path")
	}
}

func TestAllowLoadPath(t *testing.T) {
	// Should not panic
	err := AllowLoadPath("/some/path")
	_ = err
}

func TestStore_IsTrusted(t *testing.T) {
	s := &Store{
		Entries: map[string]Entry{
			"/trusted": {Path: "/trusted", TrustedAt: time.Now()},
		},
	}

	if !s.IsTrusted("/trusted") {
		t.Error("expected /trusted to be trusted")
	}
	if s.IsTrusted("/nonexistent") {
		t.Error("expected /nonexistent to NOT be trusted")
	}
}

func TestStore_Trust(t *testing.T) {
	// Trust requires a file path for saving
	tmpFile := filepath.Join(t.TempDir(), "trust.json")
	s, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	err = s.Trust("/test/path", "test reason")
	if err != nil {
		t.Fatalf("Trust error: %v", err)
	}
	if !s.IsTrusted("/test/path") {
		t.Error("expected /test/path to be trusted after Trust()")
	}
}

func TestStore_Untrust(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "trust.json")
	s, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	err = s.Trust("/test/path", "test reason")
	if err != nil {
		t.Fatalf("Trust error: %v", err)
	}
	err = s.Untrust("/test/path")
	if err != nil {
		t.Fatalf("Untrust error: %v", err)
	}
	if s.IsTrusted("/test/path") {
		t.Error("expected /test/path to NOT be trusted after Untrust()")
	}
}

func TestStore_Untrust_NonExistent(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "trust.json")
	s, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	err = s.Untrust("/nonexistent")
	if err != nil {
		t.Errorf("Untrust on non-existent should not error: %v", err)
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	if path == "" {
		t.Error("expected non-empty default path")
	}
}

func TestEnabled(t *testing.T) {
	// Should not panic
	_ = Enabled()
}
