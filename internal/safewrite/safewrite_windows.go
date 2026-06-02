//go:build windows

package safewrite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathEscape    = errors.New("safewrite: path escapes parent directory")
	ErrSymlinkTarget = errors.New("safewrite: destination is a symlink")
)

func WriteFile(path string, data []byte) error {
	if path == "" {
		return errors.New("safewrite: empty path")
	}
	if data == nil {
		return errors.New("safewrite: nil data")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return err
		}
		cleaned = abs
	}
	if strings.Contains(cleaned, "..") {
		return ErrPathEscape
	}
	linfo, err := os.Lstat(cleaned)
	if err == nil && linfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("safewrite: refusing to write through symlink")
	}
	return os.WriteFile(cleaned, data, 0o600)
}
