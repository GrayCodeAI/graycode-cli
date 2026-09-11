// Package identity provides a per-harness-home anonymous user id shared by
// telemetry and feedback.
//
// The id is a random UUID persisted as a bare line in `.anonymous-user-id`
// inside the harness home (`$HAWK_HOME` > `~/.hawk`), and never derived from
// the hostname, network address, git remote, or any other identifying source.
// It is scoped to the harness home, not the machine: every process sharing one
// home reports the same id, and deleting the file mints a fresh identity on
// the next launch.
//
// Ported from DSH `identity/anonymous-user-id` (dsh-v0.1.0-rc.7).
package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// fileName is the bare-line file holding the id, matching DSH's
// `.anonymous-user-id`.
const fileName = ".anonymous-user-id"

// overrideHome is a test seam; when non-empty it replaces home resolution.
// Guarded by stateMu because it is mutated only from tests.
var (
	stateMu      sync.Mutex
	overrideHome string
	memo         = make(map[string]*Identity)
)

// SetHomeDir overrides the harness home used by Resolve. It exists so tests
// never touch the real `~/.hawk`. Pass "" to clear the override.
func SetHomeDir(dir string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	overrideHome = dir
}

// HomeDir resolves the harness home: `$HAWK_HOME` when set, otherwise
// `~/.hawk`. DSH uses `$DSH_HOME > ~/.dsh`; hawk mirrors that convention.
func HomeDir() (string, error) {
	stateMu.Lock()
	over := overrideHome
	stateMu.Unlock()
	if over != "" {
		return over, nil
	}
	if env := strings.TrimSpace(os.Getenv("HAWK_HOME")); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("identity: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".hawk"), nil
}

// Identity is one resolved anonymous user id.
type Identity struct {
	mu   sync.Mutex
	path string
	id   string
}

// ID returns the anonymous user id, minting and persisting it on first use.
func (i *Identity) ID() (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.id != "" {
		return i.id, nil
	}
	id, err := loadOrMint(i.path)
	if err != nil {
		return "", err
	}
	i.id = id
	return id, nil
}

// FilePath returns the resolved `.anonymous-user-id` path.
func (i *Identity) FilePath() string { return i.path }

// Resolve returns the memoized identity for the resolved harness home. The
// result is memoized per resolved file path, so one process reports one id
// even across concurrent Resolve callers.
func Resolve() (*Identity, error) {
	home, err := HomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, fileName)
	stateMu.Lock()
	defer stateMu.Unlock()
	if id, ok := memo[path]; ok {
		return id, nil
	}
	id := &Identity{path: path}
	memo[path] = id
	return id, nil
}

// loadOrMint reads the bare-line id from path, creating the file with a fresh
// UUID when absent. Reads and writes are synchronous so boot-time consumers
// can use one API.
func loadOrMint(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("identity: read %s: %w", path, err)
	}
	id := uuid.NewString()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("identity: create home %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("identity: write %s: %w", path, err)
	}
	return id, nil
}
