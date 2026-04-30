package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadLatestForCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()

	old := &Session{ID: "old", CWD: cwd, UpdatedAt: time.Now().Add(-time.Hour), Messages: []Message{{Role: "user", Content: "old"}}}
	newer := &Session{ID: "new", CWD: cwd, UpdatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "new"}}}
	if err := Save(old); err != nil {
		t.Fatal(err)
	}
	if err := Save(newer); err != nil {
		t.Fatal(err)
	}

	got, err := LoadLatestForCWD(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "new" {
		t.Fatalf("got %q, want new", got.ID)
	}
}

func TestSaveFillsCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(cwd)
	defer os.Chdir(orig)

	if err := Save(&Session{ID: "session"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load("session")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.Getwd()
	want, _ = filepath.Abs(want)
	if got.CWD != want {
		t.Fatalf("got cwd %q, want %q", got.CWD, want)
	}
}
