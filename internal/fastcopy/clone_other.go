//go:build !darwin

package fastcopy

// tryCloneFilePlatform reports unsupported; callers fall back to byte copies.
func tryCloneFilePlatform(src, dst string) (int64, bool) {
	return 0, false
}
