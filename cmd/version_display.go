package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// DisplayVersion returns the user-facing version string for banners and /version.
// Release builds inject version via ldflags; local builds fall back to VERSION file.
func DisplayVersion() string {
	v := strings.TrimSpace(version)
	if v != "" && v != "dev" {
		return v
	}
	if fromFile := readRepoVERSIONFile(); fromFile != "" {
		return fromFile
	}
	if v != "" {
		return v
	}
	return "dev"
}

func readRepoVERSIONFile() string {
	candidates := versionFileCandidates()
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(data))
		if v != "" {
			return v
		}
	}
	return ""
}

func versionFileCandidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 4; i++ {
			out = append(out, filepath.Join(dir, "VERSION"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 4; i++ {
			out = append(out, filepath.Join(dir, "VERSION"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return out
}
