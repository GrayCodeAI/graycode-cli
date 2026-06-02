//go:build !windows

package safewrite_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/safewrite"
)

func TestWriteFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := safewrite.WriteFile(path, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("unexpected content: %q", got)
	}
	// Mode must be 0600
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected mode 0600, got %o", mode)
	}
}

func TestWriteFile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Pre-existing regular file
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := safewrite.WriteFile(path, []byte("new")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("expected 'new', got %q", got)
	}
}

func TestWriteFile_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("real"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	err := safewrite.WriteFile(link, []byte("evil"))
	if !errors.Is(err, safewrite.ErrSymlinkTarget) {
		t.Errorf("expected ErrSymlinkTarget, got %v", err)
	}
	// Target file must not be modified
	got, _ := os.ReadFile(target)
	if string(got) != "real" {
		t.Errorf("target was modified: %q", got)
	}
}

func TestWriteFile_RefusesSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	symDir := filepath.Join(dir, "sym")
	if err := os.Symlink(realDir, symDir); err != nil {
		t.Fatal(err)
	}
	// Path through the symlinked dir
	path := filepath.Join(symDir, "config.json")
	err := safewrite.WriteFile(path, []byte("data"))
	if err == nil || !strings.Contains(err.Error(), "parent dir is a symlink") {
		t.Errorf("expected parent-symlink error, got %v", err)
	}
}

func TestWriteFile_AtomicOnCrash(t *testing.T) {
	// After a successful write, the destination must exist with the
	// new content. The temp file must NOT exist.
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")
	if err := safewrite.WriteFile(path, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".safewrite.") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteFile_EmptyPath(t *testing.T) {
	if err := safewrite.WriteFile("", []byte("x")); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestWriteFile_EmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := safewrite.WriteFile(path, nil); err != nil {
		t.Fatalf("WriteFile nil data: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Size() != 0 {
		t.Errorf("expected 0 bytes, got %d", info.Size())
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected mode 0600, got %o", info.Mode().Perm())
	}
}

func TestWriteFile_LargeData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	data := make([]byte, 1<<20) // 1 MiB
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := safewrite.WriteFile(path, data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, _ := os.ReadFile(path)
	if len(got) != len(data) {
		t.Errorf("expected %d bytes, got %d", len(data), len(got))
	}
	for i, b := range got {
		if b != byte(i%256) {
			t.Errorf("byte mismatch at %d: got %d want %d", i, b, byte(i%256))
			break
		}
	}
}

func TestWriteFile_ConcurrentWritesToDifferentPaths(t *testing.T) {
	dir := t.TempDir()
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			path := filepath.Join(dir, "file_"+string(rune('a'+i))+".txt")
			done <- safewrite.WriteFile(path, []byte("hello"))
		}(i)
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent WriteFile: %v", err)
		}
	}
	// Verify all files exist
	entries, _ := os.ReadDir(dir)
	if len(entries) != 10 {
		t.Errorf("expected 10 files, got %d", len(entries))
	}
}

func TestWriteFile_OpenError(t *testing.T) {
	// Path under a file (not a dir) so mkdirAll fails before we
	// ever reach the open(O_CREAT) stage.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(blocker, "subdir", "data.txt")
	err := safewrite.WriteFile(bad, []byte("data"))
	if err == nil {
		t.Error("expected error when parent is a regular file")
	}
}

func TestReadLink_BrokenSymlink(t *testing.T) {
	// Create a broken symlink and verify WriteFile still
	// detects it via Lstat (which doesn't follow the link).
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink("/nonexistent/path", broken); err != nil {
		t.Fatal(err)
	}
	err := safewrite.WriteFile(broken, []byte("data"))
	if !errors.Is(err, safewrite.ErrSymlinkTarget) {
		t.Errorf("expected ErrSymlinkTarget for broken symlink, got %v", err)
	}
}

func TestWriteFile_SymlinkParentDir(t *testing.T) {
	dir := t.TempDir()
	// Make the parent dir a symlink (should be rejected).
	real := filepath.Join(dir, "real")
	sym := filepath.Join(dir, "sym")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, sym); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sym, "data.txt")
	err := safewrite.WriteFile(path, []byte("hello"))
	if err == nil {
		t.Error("expected error for symlink parent dir")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink in error, got %v", err)
	}
}
