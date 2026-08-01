// Package fsutil provides small shared filesystem helpers.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// Exists reports whether path exists on disk (following symlinks).
// It returns false for any stat error, including permission errors.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadPinnedFile reads a file through a directory handle pinned to its
// canonical parent. This closes the symlink/rename race between path
// validation and the read operation.
func ReadPinnedFile(path string) ([]byte, error) {
	root, name, err := openPinnedParent(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(name)
}

// WritePinnedFile writes a file through a directory handle pinned to its
// canonical parent. The parent must already exist.
func WritePinnedFile(path string, data []byte, perm os.FileMode) error {
	root, name, err := openPinnedParent(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.WriteFile(name, data, perm)
}

func openPinnedParent(path string) (*os.Root, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	} else if resolvedDir, dirErr := filepath.EvalSymlinks(filepath.Dir(abs)); dirErr == nil {
		abs = filepath.Join(resolvedDir, filepath.Base(abs))
	}
	root, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return nil, "", fmt.Errorf("open pinned parent: %w", err)
	}
	return root, filepath.Base(abs), nil
}
