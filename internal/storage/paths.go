package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const (
	appName           = "hawk"
	envConfigDir      = "HAWK_CONFIG_DIR"
	envEyrieConfigDir = "EYRIE_CONFIG_DIR"
	envStateDir       = "HAWK_STATE_DIR"
	envCacheDir       = "HAWK_CACHE_DIR"
	projectIDHashLen  = 12
)

// ConfigDir returns the per-user configuration directory for Hawk.
func ConfigDir() string {
	if dir := cleanEnvDir(envConfigDir); dir != "" {
		return dir
	}
	return filepath.Join(mustUserConfigDir(), appName)
}

// StateDir returns the per-user state directory for durable runtime data.
func StateDir() string {
	if dir := cleanEnvDir(envStateDir); dir != "" {
		return dir
	}
	if dir := cleanEnvDir("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, appName)
	}
	return filepath.Join(mustUserConfigDir(), appName, "state")
}

// CacheDir returns the per-user cache directory for disposable data.
func CacheDir() string {
	if dir := cleanEnvDir(envCacheDir); dir != "" {
		return dir
	}
	return filepath.Join(mustUserCacheDir(), appName)
}

func SettingsPath() string {
	return filepath.Join(ConfigDir(), "settings.json")
}

func ProviderConfigPath() string {
	// Eyrie owns provider routing state. Keep HAWK_CONFIG_DIR as the
	// compatibility fallback, but let Eyrie's host-neutral override win when
	// both variables are configured.
	if dir := cleanEnvDir(envEyrieConfigDir); dir != "" {
		return filepath.Join(dir, "provider.json")
	}
	return filepath.Join(ConfigDir(), "provider.json")
}

func SessionsDir() string {
	return filepath.Join(StateDir(), "sessions")
}

func PlansDir(projectRoot string) string {
	return filepath.Join(ProjectStateDir(projectRoot), "plans")
}

func DaemonRunDir() string {
	return filepath.Join(StateDir(), "run")
}

func WorkspaceSnapshotsDir() string {
	return filepath.Join(StateDir(), "snapshots")
}

func PersonasDir() string {
	return filepath.Join(StateDir(), "agents")
}

func TasteDir() string {
	return filepath.Join(StateDir(), "taste")
}

func RepoMapCacheDir(projectRoot string) string {
	return filepath.Join(ProjectCacheDir(projectRoot), "repomap")
}

func ProjectStateDir(projectRoot string) string {
	return filepath.Join(StateDir(), "projects", ProjectID(projectRoot))
}

func ProjectCacheDir(projectRoot string) string {
	return filepath.Join(CacheDir(), "projects", ProjectID(projectRoot))
}

// ProjectID returns a stable, filesystem-safe identifier for a project path.
func ProjectID(projectRoot string) string {
	if projectRoot == "" {
		projectRoot = "."
	}
	abs, err := filepath.Abs(projectRoot)
	if err == nil {
		projectRoot = abs
	}
	projectRoot = filepath.Clean(projectRoot)
	base := filepath.Base(projectRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "project"
	}
	base = sanitizeName(base)
	sum := sha256.Sum256([]byte(projectRoot))
	return base + "-" + hex.EncodeToString(sum[:])[:projectIDHashLen]
}

func cleanEnvDir(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func mustUserConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		// Never crash the CLI at init because the environment is broken
		// (e.g. unset HOME in a cron/daemon context). Fall back to a
		// stable, writable location under the OS temp dir so the process
		// still functions; the effective paths are also overridable via
		// HAWK_CONFIG_DIR / HAWK_STATE_DIR / HAWK_CACHE_DIR.
		return filepath.Join(os.TempDir(), "hawk-config")
	}
	return dir
}

func mustUserCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		return filepath.Join(os.TempDir(), "hawk-cache")
	}
	return dir
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "project"
	}
	return out
}
