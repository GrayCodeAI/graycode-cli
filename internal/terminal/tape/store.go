// Package tape: hawk-native tape status + commit checkpoint (not an fx
// feature — fx exposes no `tape status`/`tape commit`). Status summarizes a
// tape's header, frame mix, and footprint; commit copies a validated tape into
// a named immutable location with a content hash so a session can be recalled
// as an artifact.
package tape

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TapeStatus summarizes a tape's header, frame mix, and footprint.
type TapeStatus struct {
	Path        string         `json:"path"`
	Size        int64          `json:"size_bytes"`
	Cols        uint16         `json:"cols"`
	Rows        uint16         `json:"rows"`
	EpochMS     int64          `json:"epoch_ms"`
	Version     string         `json:"version"`
	FrameCount  int            `json:"frame_count"`
	ResizeCount int            `json:"resize_count"`
	StdoutBytes int            `json:"stdout_bytes"`
	DurationMS  int64          `json:"duration_ms"`
	Kinds       map[string]int `json:"kinds"`
}

// InspectFile parses a tape from path and returns its status.
func InspectFile(path string) (*TapeStatus, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	t, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("bad tape %s: %w", path, err)
	}
	st := &TapeStatus{
		Path:       path,
		Size:       info.Size(),
		Cols:       t.Header.Cols,
		Rows:       t.Header.Rows,
		EpochMS:    t.Header.EpochMS,
		Version:    t.Header.Version,
		FrameCount: len(t.Frames),
		Kinds:      map[string]int{},
	}
	for _, f := range t.Frames {
		st.Kinds[f.Kind.String()]++
		st.DurationMS += int64(f.DeltaMS)
		switch f.Kind {
		case KindResize:
			st.ResizeCount++
		case KindStdout:
			st.StdoutBytes += len(f.Payload)
		}
	}
	return st, nil
}

// TapeCommit describes a committed (checkpointed) copy of a tape.
type TapeCommit struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	MetaPath string `json:"meta_path"`
	CommitID string `json:"commit_id"` // first 12 hex chars of the content SHA-256
	Frames   int    `json:"frame_count"`
}

// commitMeta is written next to every committed tape.
type commitMeta struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	CommitID   string `json:"commit_id"`
	Committed  string `json:"committed"` // RFC3339 UTC
	FrameCount int    `json:"frame_count"`
	Size       int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
}

// DefaultTapesDir returns where committed tapes are stored by default.
func DefaultTapesDir() string {
	if d := os.Getenv("HAWK_TAPES_DIR"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "hawk", "tapes")
}

// ValidCommitName reports whether name is a safe tape commit filename.
func ValidCommitName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			continue
		default:
			return false
		}
	}
	return true
}

// CommitFile validates src as a tape, then copies it into dir as
// <name>.fxtape (failing if it already exists) with a sidecar
// <name>.meta.json. An empty dir uses DefaultTapesDir.
func CommitFile(srcPath, name, dir string) (*TapeCommit, error) {
	if !ValidCommitName(name) {
		return nil, fmt.Errorf("invalid tape name %q", name)
	}
	if dir == "" {
		dir = DefaultTapesDir()
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcPath, err)
	}
	t, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("bad tape %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("make commit dir: %w", err)
	}

	tapePath := filepath.Join(dir, name+".fxtape")
	f, err := os.OpenFile(tapePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("tape %q already committed at %s", name, tapePath)
		}
		return nil, fmt.Errorf("create %s: %w", tapePath, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write %s: %w", tapePath, err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	sha := sha256.Sum256(data)
	meta := commitMeta{
		Name:       name,
		Source:     srcPath,
		CommitID:   hex.EncodeToString(sha[:6]),
		Committed:  time.Now().UTC().Format(time.RFC3339),
		FrameCount: len(t.Frames),
		Size:       int64(len(data)),
		SHA256:     hex.EncodeToString(sha[:]),
	}
	mraw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	metaPath := filepath.Join(dir, name+".meta.json")
	if err := os.WriteFile(metaPath, append(mraw, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", metaPath, err)
	}
	return &TapeCommit{
		Name:     name,
		Path:     tapePath,
		MetaPath: metaPath,
		CommitID: meta.CommitID,
		Frames:   len(t.Frames),
	}, nil
}
