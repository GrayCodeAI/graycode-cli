package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Snapshot records a point-in-time copy of a session.
type Snapshot struct {
	ID        int       `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	MsgIndex  int       `json:"msg_index"`
	Action    string    `json:"action"`
	Label     string    `json:"label,omitempty"`
}

// SnapshotStore manages snapshots for a session.
type SnapshotStore struct {
	sessionID string
	snapshots []Snapshot
	dir       string
	maxSnaps  int
}

// NewSnapshotStore creates a new snapshot store for the given session.
func NewSnapshotStore(sessionID string) *SnapshotStore {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".hawk", "sessions", sessionID, "snapshots")
	return &SnapshotStore{
		sessionID: sessionID,
		dir:       dir,
		maxSnaps:  50,
	}
}

// Take saves a snapshot of the current session state.
func (ss *SnapshotStore) Take(action string, sess *Session) error {
	if err := os.MkdirAll(ss.dir, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	nextID := 1
	if len(ss.snapshots) > 0 {
		nextID = ss.snapshots[len(ss.snapshots)-1].ID + 1
	}

	snap := Snapshot{
		ID:        nextID,
		Timestamp: time.Now(),
		MsgIndex:  len(sess.Messages),
		Action:    action,
	}

	// Save a copy of the session as a JSONL file
	snapPath := filepath.Join(ss.dir, fmt.Sprintf("%d.jsonl", nextID))
	if err := writeSessionJSONL(snapPath, sess); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	ss.snapshots = append(ss.snapshots, snap)

	// Persist the index
	if err := ss.saveIndex(); err != nil {
		return fmt.Errorf("save snapshot index: %w", err)
	}

	ss.Cleanup()
	return nil
}

// List returns all snapshots, oldest first.
func (ss *SnapshotStore) List() []Snapshot {
	out := make([]Snapshot, len(ss.snapshots))
	copy(out, ss.snapshots)
	return out
}

// Rewind restores the session to the state at the given snapshot ID.
func (ss *SnapshotStore) Rewind(id int) (*Session, error) {
	snapPath := filepath.Join(ss.dir, fmt.Sprintf("%d.jsonl", id))
	if _, err := os.Stat(snapPath); err != nil {
		return nil, fmt.Errorf("snapshot %d not found", id)
	}

	return loadSnapshotJSONL(snapPath, ss.sessionID)
}

// Load reads the snapshot index from disk.
func (ss *SnapshotStore) Load() error {
	if err := os.MkdirAll(ss.dir, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	indexPath := filepath.Join(ss.dir, "snapshots.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			ss.snapshots = nil
			return nil
		}
		return fmt.Errorf("read snapshot index: %w", err)
	}

	var snaps []Snapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		return fmt.Errorf("parse snapshot index: %w", err)
	}
	ss.snapshots = snaps
	return nil
}

// Format returns a human-readable list of snapshots.
func (ss *SnapshotStore) Format() string {
	if len(ss.snapshots) == 0 {
		return "No snapshots."
	}

	result := fmt.Sprintf("Snapshots (%d):\n", len(ss.snapshots))
	for _, s := range ss.snapshots {
		label := ""
		if s.Label != "" {
			label = fmt.Sprintf(" [%s]", s.Label)
		}
		result += fmt.Sprintf("  #%d  %s  %s  (%d msgs)%s\n",
			s.ID,
			s.Timestamp.Format("2006-01-02 15:04:05"),
			s.Action,
			s.MsgIndex,
			label,
		)
	}
	return result
}

// Cleanup removes old snapshots, keeping only the most recent maxSnaps.
func (ss *SnapshotStore) Cleanup() {
	if len(ss.snapshots) <= ss.maxSnaps {
		return
	}

	// Sort by ID ascending to know which to remove
	sort.Slice(ss.snapshots, func(i, j int) bool {
		return ss.snapshots[i].ID < ss.snapshots[j].ID
	})

	toRemove := ss.snapshots[:len(ss.snapshots)-ss.maxSnaps]
	ss.snapshots = ss.snapshots[len(ss.snapshots)-ss.maxSnaps:]

	// Remove old snapshot files
	for _, s := range toRemove {
		path := filepath.Join(ss.dir, fmt.Sprintf("%d.jsonl", s.ID))
		os.Remove(path)
	}

	// Update index
	ss.saveIndex()
}

// saveIndex writes the snapshot index to disk.
func (ss *SnapshotStore) saveIndex() error {
	indexPath := filepath.Join(ss.dir, "snapshots.json")
	data, err := json.MarshalIndent(ss.snapshots, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0o644)
}

// writeSessionJSONL writes a session as JSONL to the given path.
func writeSessionJSONL(path string, sess *Session) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write metadata
	meta := map[string]interface{}{
		"type":       "session_meta",
		"id":         sess.ID,
		"model":      sess.Model,
		"provider":   sess.Provider,
		"cwd":        sess.CWD,
		"name":       sess.Name,
		"created_at": sess.CreatedAt.Format(time.RFC3339),
		"updated_at": sess.UpdatedAt.Format(time.RFC3339),
	}
	metaData, _ := json.Marshal(meta)
	f.Write(metaData)
	f.Write([]byte("\n"))

	// Write messages
	for _, msg := range sess.Messages {
		msgData, _ := json.Marshal(msg)
		f.Write(msgData)
		f.Write([]byte("\n"))
	}

	return f.Sync()
}

// loadSnapshotJSONL reads a session from a snapshot JSONL file.
func loadSnapshotJSONL(path, sessionID string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	sess := &Session{ID: sessionID}
	lines := splitSnapshotLines(data)
	firstLine := true

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		if firstLine {
			firstLine = false
			var meta map[string]interface{}
			if err := json.Unmarshal(line, &meta); err != nil {
				return nil, fmt.Errorf("parse snapshot meta: %w", err)
			}
			if v, ok := meta["model"].(string); ok {
				sess.Model = v
			}
			if v, ok := meta["provider"].(string); ok {
				sess.Provider = v
			}
			if v, ok := meta["cwd"].(string); ok {
				sess.CWD = v
			}
			if v, ok := meta["name"].(string); ok {
				sess.Name = v
			}
			if v, ok := meta["created_at"].(string); ok {
				sess.CreatedAt, _ = time.Parse(time.RFC3339, v)
			}
			if v, ok := meta["updated_at"].(string); ok {
				sess.UpdatedAt, _ = time.Parse(time.RFC3339, v)
			}
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		sess.Messages = append(sess.Messages, msg)
	}

	return sess, nil
}

// splitSnapshotLines splits raw bytes into lines.
func splitSnapshotLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
