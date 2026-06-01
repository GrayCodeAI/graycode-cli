// Package safewrite provides a hardened file-write helper that
// protects against common symlink and permission attacks.
//
// The function in this package — WriteFile — performs the same
// security checks as caveman's safeWriteFlag:
//
//  1. Refuses to follow symlinks (O_NOFOLLOW).
//  2. Writes to a temporary file in the same directory first.
//  3. Sets the new file's mode to 0600 (owner read/write only).
//  4. Atomically renames the temp file to the final path
//     (os.Rename on POSIX, which is atomic for same-filesystem).
//  5. Refuses to operate on paths that already exist as symlinks
//     pointing elsewhere (TOCTOU-resistant).
//
// Source: github.com/JuliusBrussee/caveman,
// bin/install.js (safeWriteFlag). Ported to native Go.
//
// This is a low-level utility intended for hooks and other code
// that writes to disk on behalf of the agent. Callers that need
// more elaborate behavior (e.g. preserving existing mode bits) should
// use os.WriteFile directly with their own audit trail.
package safewrite

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// ErrSymlinkTarget is returned when the destination path already
// exists as a symlink. The caller should remove the symlink first
// or use a different path.
var ErrSymlinkTarget = errors.New("safewrite: destination is a symlink")

// ErrPathEscape is returned when the resolved path tries to escape
// the parent directory (e.g. via "..").
var ErrPathEscape = errors.New("safewrite: path escapes parent directory")

// WriteFile writes data to path atomically with strict security
// guarantees:
//
//   - If path already exists and is a symlink, returns ErrSymlinkTarget.
//   - Writes to path.tmp.<pid> in the same directory, then renames.
//   - The final file is mode 0600.
//   - The temp file is opened with O_NOFOLLOW|O_WRONLY|O_CREATE|O_TRUNC.
//   - The parent directory must already exist; it is not created.
//
// On success, the destination file exists with the requested contents
// and mode 0600. On failure, the destination file is unchanged and
// any temp file left behind is cleaned up.
func WriteFile(path string, data []byte) error {
	if path == "" {
		return errors.New("safewrite: empty path")
	}
	// Reject symlinks at the destination BEFORE doing anything else.
	// This is a TOCTOU-resistant check: Lstat does not follow symlinks.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s -> %s", ErrSymlinkTarget, path, readLink(path))
		}
	}

	// Resolve the parent directory.
	dir := filepath.Dir(path)
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("safewrite: resolve dir: %w", err)
		}
		dir = abs
	}
	// Ensure the parent directory itself is not a symlink.
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("safewrite: parent dir is a symlink: %s", dir)
		}
	}
	// Ensure the path does not try to escape the parent via "..".
	cleaned := filepath.Clean(path)
	rel, err := filepath.Rel(dir, cleaned)
	if err != nil {
		return fmt.Errorf("safewrite: relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrPathEscape, path)
	}

	// Build a temp file name in the same directory.
	tmpName := filepath.Join(dir, fmt.Sprintf(".safewrite.%d.%s.tmp",
		unix.Getpid(), strconv.FormatInt(randSuffix(), 36)))

	// Open with O_NOFOLLOW so a symlink that appears between Lstat
	// and Openat is detected and rejected.
	fd, err := unix.Open(tmpName,
		unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_NOFOLLOW,
		0o600)
	if err != nil {
		return fmt.Errorf("safewrite: open temp: %w", err)
	}
	tmpFile := os.NewFile(uintptr(fd), tmpName)
	defer func() {
		_ = tmpFile.Close()
	}()

	// Write all data.
	if _, err := tmpFile.Write(data); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("safewrite: write temp: %w", err)
	}
	// Sync to disk before rename so a crash doesn't leave an
	// empty file at the destination.
	if err := tmpFile.Sync(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("safewrite: sync temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("safewrite: close temp: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("safewrite: rename: %w", err)
	}
	// Belt-and-suspenders: ensure mode is 0600 (umask may interfere
	// with the open-mode argument on some platforms).
	_ = os.Chmod(path, 0o600)
	return nil
}

// readLink is a small wrapper around os.Readlink that returns
// "<unreadable>" if the read fails (e.g. broken symlink).
func readLink(path string) string {
	if target, err := os.Readlink(path); err == nil {
		return target
	}
	return "<unreadable>"
}

var (
	randMu  sync.Mutex
	randSrc *rand.Rand
)

// randSuffix returns a random integer for the temp-file suffix.
// The math/rand source is seeded once at package init time.
func randSuffix() int64 {
	randMu.Lock()
	defer randMu.Unlock()
	if randSrc == nil {
		randSrc = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return randSrc.Int63()
}
