// Package fsutil provides small shared filesystem helpers.
package fsutil

import "os"

// Exists reports whether path exists on disk (following symlinks).
// It returns false for any stat error, including permission errors.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
