package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// versionLine is the single user-facing version format shared by
// `hawk --version` and `hawk version`.
func versionLine() string {
	ver := DisplayVersion()
	if ver != "" && !strings.HasPrefix(ver, "v") && !strings.HasPrefix(ver, "V") {
		ver = "v" + ver
	}
	line := "hawk " + ver
	if d := strings.TrimSpace(buildDate); d != "" && d != "unknown" {
		line += " (built " + d + ")"
	}
	return line
}

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
		data, err := os.ReadFile(path) // #nosec G304 -- path from internal candidate list, not external input
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
