package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHawkMD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HAWK.md"), []byte("test instructions"), 0o644)

	// Change to temp dir
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	md := LoadHawkMD()
	if md != "test instructions" {
		t.Fatalf("got %q", md)
	}
}

func TestLoadHawkMDMissing(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	md := LoadHawkMD()
	if md != "" {
		t.Fatalf("expected empty, got %q", md)
	}
}

func TestBuildContext(t *testing.T) {
	ctx := BuildContext()
	if !strings.Contains(ctx, "Working directory:") {
		t.Fatal("expected Working directory in context")
	}
}
