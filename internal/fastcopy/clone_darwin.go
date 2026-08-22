//go:build darwin

package fastcopy

import (
	"os"

	"golang.org/x/sys/unix"
)

// tryCloneFilePlatform clones src into dst via APFS clonefile.
func tryCloneFilePlatform(src, dst string) (int64, bool) {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		return 0, false
	}
	info, err := os.Stat(src)
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}
