package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

func validatePathAllowed(ctx context.Context, path string) error {
	tc := GetToolContext(ctx)
	if tc == nil {
		return nil
	}
	// Explicitly selecting the off sandbox disables the workspace path guard.
	// An unset mode is intentionally not treated as off so direct tool callers
	// retain the safe historical default.
	if tc.SandboxMode == sandbox.ModeOff {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	absPath, err := guardedAbs(path)
	if err != nil {
		return err
	}
	roots := append([]string{"."}, tc.AllowedDirectories...)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absRoot, err := guardedAbs(root)
		if err != nil {
			continue
		}
		if sameOrWithin(absPath, absRoot) {
			return nil
		}
	}
	return fmt.Errorf("path %s is outside the working directory and allowed directories; use --add-dir or /add-dir to allow it", absPath)
}

func guardedAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	dir, base := filepath.Dir(abs), filepath.Base(abs)
	if resolvedDir, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(resolvedDir, base), nil
	}
	return abs, nil
}

// guardedRootPath opens the canonical parent directory of a path and returns
// the basename to use with os.Root. The permission check happens before the
// root is opened; os.Root then pins the directory handle and closes the
// symlink/rename race between validation and the actual file operation.
func guardedRootPath(ctx context.Context, path string) (*os.Root, string, error) {
	if err := validatePathAllowed(ctx, path); err != nil {
		return nil, "", err
	}
	absPath, err := guardedAbs(path)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(filepath.Dir(absPath))
	if err != nil {
		return nil, "", fmt.Errorf("open guarded parent: %w", err)
	}
	return root, filepath.Base(absPath), nil
}

func readGuardedFile(ctx context.Context, path string) ([]byte, error) {
	root, name, err := guardedRootPath(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(name)
}

func writeGuardedFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	root, name, err := guardedRootPath(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.WriteFile(name, data, perm)
}

// readPinnedFile and writePinnedFile are for lower-level tool APIs that do not
// carry a context. They still pin the canonical parent directory with os.Root,
// preventing a symlink swap between path resolution and the file operation.
func readPinnedFile(path string) ([]byte, error) {
	return readGuardedFile(context.Background(), path)
}

func writePinnedFile(path string, data []byte, perm os.FileMode) error {
	return writeGuardedFile(context.Background(), path, data, perm)
}

// ReadPinnedFile exposes the same root-pinned read for engine components that
// already depend on the tool package but do not carry a ToolContext.
func ReadPinnedFile(path string) ([]byte, error) { return readPinnedFile(path) }

// WritePinnedFile exposes the same root-pinned write for engine components.
func WritePinnedFile(path string, data []byte, perm os.FileMode) error {
	return writePinnedFile(path, data, perm)
}

func sameOrWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if sameFilePath(path, root) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func sameFilePath(a, b string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
