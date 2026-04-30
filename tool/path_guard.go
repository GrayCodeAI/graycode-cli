package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validatePathAllowed(ctx context.Context, path string) error {
	tc := GetToolContext(ctx)
	if tc == nil {
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
