package plugin

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/GrayCodeAI/graycode-cli/internal/fsutil"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
	"github.com/GrayCodeAI/graycode-cli/internal/trust"
)

// Scope identifies a plugin discovery scope.
type Scope string

const (
	// ScopeManaged is org/IT-managed plugins (highest priority).
	ScopeManaged Scope = "managed"
	// ScopeUser is the per-user plugins directory.
	ScopeUser Scope = "user"
	// ScopeProject is project-local .graycode/plugins (requires folder trust).
	ScopeProject Scope = "project"
)

// ScopePriority returns lower number = higher priority (managed wins).
func ScopePriority(s Scope) int {
	switch s {
	case ScopeManaged:
		return 0
	case ScopeUser:
		return 1
	case ScopeProject:
		return 2
	default:
		return 99
	}
}

// ScopeDir is a discovered plugin root under a scope.
type ScopeDir struct {
	Scope Scope
	Path  string
}

// DiscoverScopeDirs returns plugin directories ordered by priority (managed first).
// Project dirs are omitted when folder trust denies them.
func DiscoverScopeDirs(projectRoot string) []ScopeDir {
	var out []ScopeDir

	// Managed: GRAYCODE_MANAGED_PLUGINS or config managed/plugins
	if m := os.Getenv("GRAYCODE_MANAGED_PLUGINS"); m != "" {
		for _, p := range filepath.SplitList(m) {
			if p == "" {
				continue
			}
			if fsutil.Exists(p) {
				out = append(out, ScopeDir{Scope: ScopeManaged, Path: p})
			}
		}
	}
	managedDefault := filepath.Join(storage.ConfigDir(), "managed", "plugins")
	if st, err := os.Stat(managedDefault); err == nil && st.IsDir() {
		out = append(out, ScopeDir{Scope: ScopeManaged, Path: managedDefault})
	}

	// User
	userDir := filepath.Join(storage.StateDir(), "plugins")
	out = append(out, ScopeDir{Scope: ScopeUser, Path: userDir})

	// Project
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	if projectRoot != "" {
		proj := filepath.Join(projectRoot, ".graycode", "plugins")
		if st, err := os.Stat(proj); err == nil && st.IsDir() {
			if err := trust.AllowLoadPath(proj); err == nil {
				out = append(out, ScopeDir{Scope: ScopeProject, Path: proj})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return ScopePriority(out[i].Scope) < ScopePriority(out[j].Scope)
	})
	return out
}

// ResolvePluginDirs returns the list of plugin directory paths for DynamicPluginManager.
func ResolvePluginDirs(projectRoot string) []string {
	scopes := DiscoverScopeDirs(projectRoot)
	paths := make([]string, 0, len(scopes))
	for _, s := range scopes {
		paths = append(paths, s.Path)
	}
	return paths
}
