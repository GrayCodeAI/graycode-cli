package hooks

import (
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// DiscoverHookDirs returns ordered hook directories to scan for a project.
// Project paths are still subject to folder trust inside LoadHooksDir.
func DiscoverHookDirs(projectRoot string) []string {
	var dirs []string
	if projectRoot != "" {
		dirs = append(
			dirs,
			filepath.Join(projectRoot, ".graycode", "hooks"),
			filepath.Join(projectRoot, ".agents", "hooks"),
			filepath.Join(projectRoot, ".claude", "hooks"),
		)
	}
	dirs = append(dirs, filepath.Join(storage.StateDir(), "hooks"))
	dirs = append(dirs, filepath.Join(storage.ConfigDir(), "hooks"))
	return dirs
}

// LoadDiscoveredHooks loads hooks from all discoverable directories for projectRoot
// (defaults to cwd). Returns the number of directories successfully scanned.
func LoadDiscoveredHooks(projectRoot string) int {
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	n := 0
	for _, dir := range DiscoverHookDirs(projectRoot) {
		if err := LoadHooksDir(dir); err != nil {
			continue
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			n++
		}
	}
	return n
}
