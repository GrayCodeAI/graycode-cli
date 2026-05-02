package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewShadowWorkspace(t *testing.T) {
	sw, err := NewShadowWorkspace()
	if err != nil {
		t.Fatalf("NewShadowWorkspace error: %v", err)
	}
	defer sw.Close()

	if sw.TempDir() == "" {
		t.Fatal("expected non-empty temp dir")
	}

	info, err := os.Stat(sw.TempDir())
	if err != nil {
		t.Fatalf("temp dir stat error: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected temp dir to be a directory")
	}
}

func TestShadowWorkspace_ValidateEdit_ValidGo(t *testing.T) {
	sw, err := NewShadowWorkspace()
	if err != nil {
		t.Fatalf("NewShadowWorkspace error: %v", err)
	}
	defer sw.Close()

	validGo := "package main\n\nfunc main() {}\n"
	errs := sw.ValidateEdit("/fake/main.go", validGo)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid Go, got %d: %v", len(errs), errs)
	}
}

func TestShadowWorkspace_ValidateEdit_InvalidGo(t *testing.T) {
	sw, err := NewShadowWorkspace()
	if err != nil {
		t.Fatalf("NewShadowWorkspace error: %v", err)
	}
	defer sw.Close()

	invalidGo := "package main\n\nfunc main( {\n}\n"
	errs := sw.ValidateEdit("/fake/main.go", invalidGo)
	if len(errs) == 0 {
		t.Error("expected errors for invalid Go, got none")
	}
}

func TestShadowWorkspace_ValidateMultipleEdits(t *testing.T) {
	sw, err := NewShadowWorkspace()
	if err != nil {
		t.Fatalf("NewShadowWorkspace error: %v", err)
	}
	defer sw.Close()

	edits := map[string]string{
		filepath.Join(t.TempDir(), "good.go"): "package main\n\nfunc main() {}\n",
		filepath.Join(t.TempDir(), "unknown.xyz"): "some content",
	}

	results := sw.ValidateMultipleEdits(edits)
	// good.go should validate cleanly; unknown.xyz has no validator
	for path, errs := range results {
		t.Errorf("unexpected errors for %s: %v", path, errs)
	}
}

func TestShadowWorkspace_Close(t *testing.T) {
	sw, err := NewShadowWorkspace()
	if err != nil {
		t.Fatalf("NewShadowWorkspace error: %v", err)
	}
	dir := sw.TempDir()
	sw.Close()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected temp dir to be removed after Close")
	}
}
