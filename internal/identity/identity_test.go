package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// freshHome returns a per-test home dir and registers cleanup.
func freshHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { SetHomeDir("") })
	return dir
}

func TestResolveMintsAndReuses(t *testing.T) {
	home := freshHome(t)
	SetHomeDir(home)

	id1, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	got1, err := id1.ID()
	if err != nil {
		t.Fatalf("ID() error: %v", err)
	}
	if got1 == "" {
		t.Fatal("ID() returned empty id")
	}
	if !strings.Contains(got1, "-") {
		t.Fatalf("ID() = %q, want a UUID-shaped id", got1)
	}

	// A second process (fresh Identity) reads the same persisted file.
	id2, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() second error: %v", err)
	}
	got2, err := id2.ID()
	if err != nil {
		t.Fatalf("ID() second error: %v", err)
	}
	if got2 != got1 {
		t.Fatalf("ID() = %q, want stable %q across processes", got2, got1)
	}
}

func TestResolveIsMemoizedPerPath(t *testing.T) {
	home := freshHome(t)
	SetHomeDir(home)

	a, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Resolve() returned different Identity objects for the same path")
	}
}

func TestDeleteFileMintsFreshIdentity(t *testing.T) {
	home := freshHome(t)
	SetHomeDir(home)

	// File-level semantics: deleting the file mints a fresh id on the next
	// launch (a new process has an empty memo). Within one process the id is
	// memoized, matching DSH's memoized `resolveDshHome` behaviour.
	first, err := loadOrMint(filepath.Join(home, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, fileName)); err != nil {
		t.Fatal(err)
	}
	second, err := loadOrMint(filepath.Join(home, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("expected a fresh identity after file deletion")
	}
}

func TestResolveStableAcrossDeletionWithinProcess(t *testing.T) {
	home := freshHome(t)
	SetHomeDir(home)

	id, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	first, err := id.ID()
	if err != nil {
		t.Fatal(err)
	}
	// In-process deletion must not change the memoized id.
	if err := os.Remove(id.FilePath()); err != nil {
		t.Fatal(err)
	}
	second, err := id.ID()
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("in-process ID() = %q, want memoized %q", second, first)
	}
}

func TestFileFormat(t *testing.T) {
	home := freshHome(t)
	SetHomeDir(home)

	id, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := id.ID(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(id.FilePath())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got == "" {
		t.Fatal("persisted file is empty")
	}
	if strings.Contains(string(b), "\n\n") {
		t.Fatalf("persisted file has unexpected layout: %q", string(b))
	}
	// The file lives directly in the harness home.
	if filepath.Dir(id.FilePath()) != home {
		t.Fatalf("FilePath() = %q, want parent %q", id.FilePath(), home)
	}
}

func TestHomeDirEnvOverride(t *testing.T) {
	freshHome(t)
	t.Setenv("GRAYCODE_HOME", "")
	home, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Without GRAYCODE_HOME the resolved home ends in /.graycode.
	if !strings.HasSuffix(home, ".graycode") {
		t.Fatalf("HomeDir() = %q, want ~/.graycode default", home)
	}
}

func TestHomeDirUsesGRAYCODEHomeEnv(t *testing.T) {
	freshHome(t)
	t.Setenv("GRAYCODE_HOME", "/tmp/graycode-home-test")
	home, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home != "/tmp/graycode-home-test" {
		t.Fatalf("HomeDir() = %q, want GRAYCODE_HOME value", home)
	}
}

func TestSetHomeDirOverrideWins(t *testing.T) {
	home := freshHome(t)
	t.Setenv("GRAYCODE_HOME", "/tmp/ignored-home")
	SetHomeDir(home)
	got, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Fatalf("HomeDir() = %q, want override %q", got, home)
	}
}
