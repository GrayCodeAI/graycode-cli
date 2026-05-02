package engine

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestNewProtectedPaths(t *testing.T) {
	pp := NewProtectedPaths()
	if pp == nil {
		t.Fatal("NewProtectedPaths returned nil")
	}
	if len(pp.List()) != 0 {
		t.Fatalf("expected empty list, got %v", pp.List())
	}
}

func TestAddAndIsProtected(t *testing.T) {
	pp := NewProtectedPaths()
	pp.Add("/etc/config.yaml")

	if !pp.IsProtected("/etc/config.yaml") {
		t.Error("expected /etc/config.yaml to be protected")
	}
	if pp.IsProtected("/etc/other.yaml") {
		t.Error("expected /etc/other.yaml to NOT be protected")
	}
}

func TestRemove(t *testing.T) {
	pp := NewProtectedPaths()
	pp.Add("/foo/bar")
	pp.Remove("/foo/bar")

	if pp.IsProtected("/foo/bar") {
		t.Error("expected /foo/bar to no longer be protected after Remove")
	}
}

func TestIsProtectedDirectory(t *testing.T) {
	pp := NewProtectedPaths()
	pp.Add("/project/vendor")

	// Files inside a protected directory should also be protected.
	if !pp.IsProtected("/project/vendor/lib.go") {
		t.Error("expected /project/vendor/lib.go to be protected (parent dir is protected)")
	}
	// The directory itself should be protected.
	if !pp.IsProtected("/project/vendor") {
		t.Error("expected /project/vendor to be protected")
	}
	// A sibling path should not be protected.
	if pp.IsProtected("/project/src/main.go") {
		t.Error("expected /project/src/main.go to NOT be protected")
	}
}

func TestIsProtectedCleansPaths(t *testing.T) {
	pp := NewProtectedPaths()
	pp.Add("/a/b/../c")

	clean := filepath.Clean("/a/b/../c")
	if !pp.IsProtected(clean) {
		t.Errorf("expected cleaned path %s to be protected", clean)
	}
}

func TestList(t *testing.T) {
	pp := NewProtectedPaths()
	pp.Add("/z/file")
	pp.Add("/a/file")

	list := pp.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(list))
	}
	// List should be sorted.
	expected := []string{filepath.Clean("/a/file"), filepath.Clean("/z/file")}
	sort.Strings(expected)
	for i, p := range list {
		if p != expected[i] {
			t.Errorf("list[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestFormatEmpty(t *testing.T) {
	pp := NewProtectedPaths()
	if pp.Format() != "" {
		t.Errorf("expected empty Format for no paths, got %q", pp.Format())
	}
}

func TestFormatNonEmpty(t *testing.T) {
	pp := NewProtectedPaths()
	pp.Add("/readonly/file.go")

	out := pp.Format()
	if !strings.Contains(out, "READ-ONLY") {
		t.Errorf("expected Format to contain READ-ONLY, got %q", out)
	}
	if !strings.Contains(out, "/readonly/file.go") {
		t.Errorf("expected Format to list the path, got %q", out)
	}
}

func TestConcurrentAccess(t *testing.T) {
	pp := NewProtectedPaths()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 1000; i++ {
			pp.Add("/concurrent/a")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 1000; i++ {
			pp.IsProtected("/concurrent/a")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 1000; i++ {
			pp.List()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	<-done
}
