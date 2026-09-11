package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/safewrite"
)

// ─────────────────────────────────────────────────────────────────────────────
// Named session checkpoints — snapshot+restore a full session (messages, model,
// provider, cwd) by a human-friendly label. This is additive on top of the
// existing CheckpointManager (which tracks point-in-time message/file state for
// rollback): named checkpoints persist a complete, self-contained session so it
// can be resumed later with `hawk resume <name>`.
// ─────────────────────────────────────────────────────────────────────────────

// NamedCheckpoint is the durable on-disk record of a labeled session snapshot.
type NamedCheckpoint struct {
	Name      string    `json:"name"`
	Session   *Session  `json:"session"`
	CreatedAt time.Time `json:"created_at"`
}

// namedCheckpointsDir is where labeled session snapshots live.
func namedCheckpointsDir() string {
	return filepath.Join(sessionsDir(), "checkpoints")
}

// sanitizeLabel converts a user label into a filesystem-safe file stem. It is
// deterministic so the same label always maps to the same file (overwrite on
// re-save), and reversible enough that List can recover the original name from
// the stored record rather than the filename.
func sanitizeLabel(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "checkpoint"
	}
	return out
}

func namedCheckpointPath(name string) string {
	return filepath.Join(namedCheckpointsDir(), sanitizeLabel(name)+".json")
}

// SaveNamedCheckpoint snapshots a full session under a human-friendly label,
// persisting it atomically. Saving with an existing label overwrites it. The
// session is deep-copied via JSON round-trip so later mutations to the live
// session do not affect the snapshot.
func SaveNamedCheckpoint(name string, s *Session) (*NamedCheckpoint, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("checkpoint name must not be empty")
	}
	if s == nil {
		return nil, fmt.Errorf("cannot checkpoint a nil session")
	}

	// Deep copy the session so the snapshot is immutable.
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal session: %w", err)
	}
	var snap Session
	if unmarshalErr := json.Unmarshal(raw, &snap); unmarshalErr != nil {
		return nil, fmt.Errorf("copy session: %w", unmarshalErr)
	}

	cp := &NamedCheckpoint{
		Name:      strings.TrimSpace(name),
		Session:   &snap,
		CreatedAt: time.Now(),
	}

	dir := namedCheckpointsDir()
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		return nil, fmt.Errorf("create checkpoints directory: %w", mkErr)
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint: %w", err)
	}

	target := namedCheckpointPath(name)
	// safewrite replaces the hand-rolled tmp+rename dance with the hardened
	// atomic writer (same 0600 mode, plus symlink refusal and fsync).
	if err := safewrite.WriteFile(target, data); err != nil {
		return nil, fmt.Errorf("write checkpoint: %w", err)
	}
	return cp, nil
}

// LoadNamedCheckpoint restores a previously saved labeled session snapshot.
func LoadNamedCheckpoint(name string) (*NamedCheckpoint, error) {
	data, err := os.ReadFile(namedCheckpointPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint %q not found", name)
		}
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp NamedCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint %q: %w", name, err)
	}
	return &cp, nil
}

// DeleteNamedCheckpoint removes a labeled session snapshot.
func DeleteNamedCheckpoint(name string) error {
	if err := os.Remove(namedCheckpointPath(name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("checkpoint %q not found", name)
		}
		return err
	}
	return nil
}

// ListNamedCheckpoints returns all labeled session snapshots, newest first.
func ListNamedCheckpoints() ([]*NamedCheckpoint, error) {
	dir := namedCheckpointsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*NamedCheckpoint
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- path built from internal named-checkpoints dir + directory entry name from os.ReadDir
		if err != nil {
			continue
		}
		var cp NamedCheckpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue
		}
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
