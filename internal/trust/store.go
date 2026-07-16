// Package trust implements folder trust for project automation (Year 0 PACK-03).
//
// Untrusted project directories must not load project-scoped hooks, MCP servers,
// LSP configs, or plugins. User-global state under the Hawk config/state dirs
// remains available without per-folder trust.
package trust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/flags"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Entry records when and why a path was trusted.
type Entry struct {
	Path      string    `json:"path"`
	TrustedAt time.Time `json:"trusted_at"`
	Reason    string    `json:"reason,omitempty"`
}

// Store is the on-disk trust database.
type Store struct {
	mu      sync.Mutex
	path    string
	Entries map[string]Entry `json:"entries"`
}

// DefaultPath returns the trust store path under Hawk state.
func DefaultPath() string {
	return filepath.Join(storage.StateDir(), "folder-trust.json")
}

// Open loads the trust store from path (or DefaultPath). Missing file is empty.
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	s := &Store{path: path, Entries: make(map[string]Entry)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("trust store: %w", err)
	}
	if s.Entries == nil {
		s.Entries = make(map[string]Entry)
	}
	s.path = path
	return s, nil
}

// Save writes the store to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// Trust marks path (and its canonical form) as trusted.
func (s *Store) Trust(path, reason string) error {
	abs, err := canonicalize(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.Entries[abs] = Entry{Path: abs, TrustedAt: time.Now().UTC(), Reason: reason}
	s.mu.Unlock()
	return s.Save()
}

// Untrust removes path from the trust store.
func (s *Store) Untrust(path string) error {
	abs, err := canonicalize(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.Entries, abs)
	s.mu.Unlock()
	return s.Save()
}

// IsTrusted reports whether path or an ancestor is in the trust store.
func (s *Store) IsTrusted(path string) bool {
	abs, err := canonicalize(path)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for p := abs; ; {
		if _, ok := s.Entries[p]; ok {
			return true
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return false
}

// List returns all trusted entries.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.Entries))
	for _, e := range s.Entries {
		out = append(out, e)
	}
	return out
}

// Enabled reports whether folder trust enforcement is active (feature flag).
func Enabled() bool {
	return flags.FolderTrust()
}

// AllowProjectAutomation is the gate for project-scoped hooks/MCP/plugins/LSP.
// When trust is disabled via flag, always allows (dev escape hatch).
// When enabled, requires the project root (or ancestor) to be trusted.
func AllowProjectAutomation(projectRoot string) error {
	if !Enabled() {
		return nil
	}
	s, err := Open("")
	if err != nil {
		return fmt.Errorf("folder trust: load store: %w", err)
	}
	if s.IsTrusted(projectRoot) {
		return nil
	}
	abs, _ := canonicalize(projectRoot)
	return fmt.Errorf("folder trust: project %q is not trusted — run `hawk trust add %s` to allow project hooks/MCP/plugins", abs, abs)
}

// IsProjectPath reports whether path is under a project tree (not Hawk user state/config).
func IsProjectPath(path string) bool {
	abs, err := canonicalize(path)
	if err != nil {
		return true // fail closed: treat unknown as project
	}
	for _, root := range []string{storage.ConfigDir(), storage.StateDir(), storage.CacheDir()} {
		r, err := canonicalize(root)
		if err != nil {
			continue
		}
		if abs == r || hasPathPrefix(abs, r) {
			return false
		}
	}
	return true
}

// RequiresFolderTrust reports whether path is a project-automation location
// that must be trusted before load (hooks/plugins/MCP under the repo).
// Generic temp dirs used in tests are not gated — only well-known project
// automation roots like <repo>/.hawk/plugins.
func RequiresFolderTrust(path string) bool {
	abs, err := canonicalize(path)
	if err != nil {
		return true
	}
	if !IsProjectPath(abs) {
		return false
	}
	sep := string(filepath.Separator)
	markers := []string{
		sep + ".hawk" + sep + "plugins",
		sep + ".hawk" + sep + "hooks",
		sep + ".hawk" + sep + "mcp",
		sep + ".hawk" + sep + "skills",
		sep + ".agents" + sep + "hooks",
		sep + ".agents" + sep + "plugins",
		sep + ".agents" + sep + "skills",
		sep + ".claude" + sep + "skills",
		sep + ".claude" + sep + "hooks",
		sep + ".codex" + sep + "skills",
		sep + ".cursor" + sep + "skills",
	}
	for _, m := range markers {
		if strings.Contains(abs, m) || strings.HasSuffix(abs, m) {
			return true
		}
		// also when path ends with .hawk/plugins etc.
		if strings.HasSuffix(abs, filepath.FromSlash(strings.TrimPrefix(m, sep))) {
			return true
		}
	}
	return false
}

// AllowLoadPath gates loading automation from path.
// User-global config/state paths always pass. Project automation roots
// (.hawk/plugins, .hawk/hooks, …) need trust when enforcement is enabled.
func AllowLoadPath(path string) error {
	if !Enabled() {
		return nil
	}
	if !RequiresFolderTrust(path) {
		return nil
	}
	return AllowProjectAutomation(path)
}

func canonicalize(path string) (string, error) {
	if path == "" {
		path, _ = os.Getwd()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Prefer EvalSymlinks when possible.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

func hasPathPrefix(path, prefix string) bool {
	rel, err := filepath.Rel(prefix, path)
	if err != nil {
		return false
	}
	return rel != ".." && !hasDotDotPrefix(rel)
}

func hasDotDotPrefix(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)
}
