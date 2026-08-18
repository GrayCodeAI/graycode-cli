package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultProjCacheVer is the current projection-cache record schema version.
// Bumping it makes Load discard old rows (never migrate), matching DSH.
const DefaultProjCacheVer = 1

// projRecord is the on-disk shape of one checkpoint row: schema version,
// committed event seq, and the folded JSON state.
type projRecord struct {
	Ver   int             `json:"ver"`
	Seq   int64           `json:"seq"`
	State json.RawMessage `json:"state"`
}

// ProjCache is a durable per-session fold checkpoint store: one record per
// session (`<seq> + ver + JSON state`) on the domain data form, landed beside
// the session JSONL (`<id>.projcache.json`).
//
// The cache is a fold shortcut, never an authority: a row is possibly stale
// (its seq says how stale) but never wrong, so every write path is fail-soft
// (a lost write costs a longer tail replay on the next cold read) and a ver
// mismatch discards the row instead of migrating it. Ported from DSH
// `session/session-projection-cache` semantics.
type ProjCache struct {
	dir string
	ver int
}

// NewProjCache returns a cache beside the session JSONL directory with the
// current schema version.
func NewProjCache() *ProjCache {
	return &ProjCache{dir: sessionsDir(), ver: DefaultProjCacheVer}
}

// NewProjCacheWithDir returns a cache rooted at dir with the current schema
// version. Tests and alternate layouts use this.
func NewProjCacheWithDir(dir string) *ProjCache {
	return &ProjCache{dir: dir, ver: DefaultProjCacheVer}
}

// NewProjCacheWithVer returns a cache rooted at dir with an explicit schema
// version (records written with a different version are discarded on Load).
func NewProjCacheWithVer(dir string, ver int) *ProjCache {
	return &ProjCache{dir: dir, ver: ver}
}

// pathFor resolves the sidecar record path for one session, refusing ids that
// could escape the cache directory (a session id is a slot name, never a
// path).
func (c *ProjCache) pathFor(id string) (string, error) {
	if id == "" || id == "." || id == ".." ||
		filepath.Base(id) != id || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("projcache: invalid session id %q", id)
	}
	return filepath.Join(c.dir, id+".projcache.json"), nil
}

// Load returns the folded state, its committed seq, and whether the row was
// fresh: identity-matching and same schema version. A missing, corrupt, or
// version-mismatched row is fail-soft — the mismatch row is discarded (never
// migrated) so a stale record can never seed state folded from an unrelated
// log.
func (c *ProjCache) Load(id string) (state []byte, seq int64, fresh bool) {
	path, err := c.pathFor(id)
	if err != nil {
		return nil, 0, false
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path sanitized by pathFor
	if err != nil {
		return nil, 0, false
	}
	var rec projRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, 0, false
	}
	if rec.Ver != c.ver {
		// Discard instead of migrating: a versioned row from another writer
		// must not seed this cache's folds.
		_ = os.Remove(path)
		return nil, 0, false
	}
	return rec.State, rec.Seq, true
}

// Save durably writes one checkpoint row for a session (atomic temp+rename).
// It is fail-soft by contract: callers may ignore the error at the cost of a
// longer replay, and a failure never corrupts the session log itself.
func (c *ProjCache) Save(id string, seq int64, state []byte) error {
	path, err := c.pathFor(id)
	if err != nil {
		return err
	}
	rec := projRecord{Ver: c.ver, Seq: seq, State: json.RawMessage(state)}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("projcache: encode: %w", err)
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("projcache: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G304 -- path sanitized by pathFor
		return fmt.Errorf("projcache: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("projcache: rename: %w", err)
	}
	return nil
}

// Discard removes the record for a session at retire. A missing record is not
// an error.
func (c *ProjCache) Discard(id string) error {
	path, err := c.pathFor(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("projcache: discard: %w", err)
	}
	return nil
}
