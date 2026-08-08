package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir(): %v", err)
	}
	if dir == "" {
		t.Fatal("Dir() returned empty path")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("Dir() = %q, want absolute path", dir)
	}
}

func TestDir_RespectsEnv(t *testing.T) {
	// os.UserHomeDir honors $HOME on Unix.
	t.Setenv("HOME", t.TempDir())
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() with HOME set: %v", err)
	}
	if dir != os.Getenv("HOME") {
		t.Errorf("Dir() = %q, want $HOME = %q", dir, os.Getenv("HOME"))
	}
}

func TestMustDir(t *testing.T) {
	dir := MustDir()
	if dir == "" {
		t.Fatal("MustDir() returned empty path")
	}
}

func TestExpand(t *testing.T) {
	homeDir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde alone", "~", homeDir},
		{"dollar home alone", "$HOME", homeDir},
		{"tilde slash prefix", "~/sub/dir", filepath.Join(homeDir, "sub", "dir")},
		{"dollar home slash prefix", "$HOME/sub/dir", filepath.Join(homeDir, "sub", "dir")},
		{"no prefix unchanged", "/abs/path", "/abs/path"},
		{"relative unchanged", "rel/path", "rel/path"},
		{"empty unchanged", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(tt.in)
			if err != nil {
				t.Fatalf("Expand(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Expand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpand_UsesHomeEnv(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("HOME", custom)
	got, err := Expand("~/x")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := filepath.Join(custom, "x")
	if got != want {
		t.Errorf("Expand(~/x) = %q, want %q", got, want)
	}
}

func TestMustExpand(t *testing.T) {
	got := MustExpand("~/must-expand")
	if !strings.HasPrefix(got, string(filepath.Separator)) && !filepath.IsAbs(got) {
		t.Errorf("MustExpand(~/must-expand) = %q, want absolute path", got)
	}
	if !strings.HasSuffix(got, "must-expand") {
		t.Errorf("MustExpand(~/must-expand) = %q, want suffix must-expand", got)
	}
}
