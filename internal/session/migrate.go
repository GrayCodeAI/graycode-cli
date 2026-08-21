package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// largeSessionBytes is the threshold above which a session is considered
// oversized and requires --allow-large to migrate (fx `session migrate` parity).
const largeSessionBytes = int64(32 << 20)

// MigrateResult describes a completed session migration.
type MigrateResult struct {
	ID          string `json:"id"`
	FromVersion int    `json:"from_version"`
	ToVersion   int    `json:"to_version"`
	SizeBytes   int64  `json:"size_bytes"`
}

// MigrateSession upgrades a saved session to the current on-disk JSONL format
// (fx `session migrate` parity). Legacy .json sessions are loaded and
// re-persisted in the current format; .jsonl sessions have their format_version
// header bumped. Oversized sessions are refused unless allowLarge is set, and a
// session already on the current format is reported without rewriting.
func MigrateSession(id string, allowLarge bool) (MigrateResult, error) {
	if err := ValidateID(id); err != nil {
		return MigrateResult{}, err
	}

	jsonl := jsonlPathFor(id)
	if _, err := os.Stat(jsonl); err == nil {
		return migrateJSONL(id, jsonl, allowLarge)
	}
	legacy := legacyPathFor(id)
	if _, err := os.Stat(legacy); err == nil {
		return migrateLegacy(id, legacy, allowLarge)
	}
	return MigrateResult{}, fmt.Errorf("session %s: %w", id, ErrNotFound)
}

// migrateLegacy converts a legacy .json session into the current JSONL format,
// preserving every message and dropping the superseded file.
func migrateLegacy(id, path string, allowLarge bool) (MigrateResult, error) {
	if err := checkSessionSize(id, path, allowLarge); err != nil {
		return MigrateResult{}, err
	}
	s, err := loadLegacyJSON(id)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("load legacy session %s: %w", id, err)
	}
	if err := Save(s); err != nil {
		return MigrateResult{}, fmt.Errorf("save migrated session %s: %w", id, err)
	}
	_ = os.Remove(path) // clean cutover: the .jsonl file now owns the session

	size := int64(0)
	if fi, err := os.Stat(jsonlPathFor(id)); err == nil {
		size = fi.Size()
	}
	return MigrateResult{ID: id, FromVersion: 0, ToVersion: SessionFormatVersion, SizeBytes: size}, nil
}

// migrateJSONL ensures a .jsonl session's header declares the current format.
func migrateJSONL(id, path string, allowLarge bool) (MigrateResult, error) {
	if err := checkSessionSize(id, path, allowLarge); err != nil {
		return MigrateResult{}, err
	}
	from, err := metaFormatVersion(path)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("read session %s format: %w", id, err)
	}
	size := int64(0)
	if fi, err := os.Stat(path); err == nil {
		size = fi.Size()
	}
	if from >= SessionFormatVersion {
		return MigrateResult{ID: id, FromVersion: from, ToVersion: SessionFormatVersion, SizeBytes: size}, nil
	}
	if err := bumpMetaVersion(path, SessionFormatVersion); err != nil {
		return MigrateResult{}, fmt.Errorf("bump session %s format: %w", id, err)
	}
	return MigrateResult{ID: id, FromVersion: from, ToVersion: SessionFormatVersion, SizeBytes: size}, nil
}

// checkSessionSize refuses oversized sessions unless allowLarge is set.
func checkSessionSize(id, path string, allowLarge bool) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat session %s: %w", id, err)
	}
	if fi.Size() > largeSessionBytes && !allowLarge {
		return fmt.Errorf("session %s is %d bytes; oversized sessions require --allow-large", id, fi.Size())
	}
	return nil
}

// metaFormatVersion reads the format_version from the session_meta header line
// of a JSONL session. A line without the key is treated as version 0.
func metaFormatVersion(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, fmt.Errorf("empty session file")
	}
	var hdr map[string]json.RawMessage
	if err := json.Unmarshal(sc.Bytes(), &hdr); err != nil {
		return 0, fmt.Errorf("invalid session_meta line: %w", err)
	}
	if v, ok := hdr["format_version"]; ok {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			return 0, err
		}
		return n, nil
	}
	return 0, nil
}

// bumpMetaVersion rewrites only the first (session_meta) line of a JSONL file,
// setting format_version to v while preserving every other byte.
func bumpMetaVersion(path string, v int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitN(string(data), "\n", 2)
	var hdr map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &hdr); err != nil {
		return fmt.Errorf("invalid session_meta line: %w", err)
	}
	hdr["format_version"] = v
	out, err := json.Marshal(hdr)
	if err != nil {
		return err
	}
	var rest string
	if len(lines) == 2 {
		rest = "\n" + lines[1]
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(string(out)+rest), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
