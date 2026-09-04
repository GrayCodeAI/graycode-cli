// Package filestate implements per-turn rewind points over the working tree,
// adopted from grok-build's checkpoint system: at each prompt boundary the
// touched files' contents are captured ("before" snapshots are first-wins,
// "after" snapshots last-write-wins), so a session can be rewound to any
// prior prompt by restoring those snapshots.
//
// A durable disk mirror (atomic temp-file writes, cap-based eviction,
// sanitized session directory names) lets rewind survive process restarts.
package filestate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ErrNoRewindPoint is returned when no checkpoint exists for an index.
var ErrNoRewindPoint = errors.New("filestate: no rewind point at index")

const (
	defaultCap       = 64
	snapshotHashLen  = 16
	storeDirPerms    = 0o750
	checkpointFormat = "checkpoint-%d.json"
)

// Snapshot is the captured content of one file at one moment.
type Snapshot struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp_unix"`
}

// RewindPoint captures file states around one prompt.
type RewindPoint struct {
	PromptIndex int `json:"prompt_index"`
	// Before maps path -> content before the first edit of this prompt
	// (first-wins within the prompt).
	Before map[string]string `json:"before,omitempty"`
	// After maps path -> content after the last edit of this prompt
	// (last-write-wins).
	After map[string]string `json:"after,omitempty"`
}

// Tracker manages rewind points for one session.
type Tracker struct {
	mu        sync.Mutex
	points    map[int]*RewindPoint
	current   *RewindPoint
	order     []int // ascending prompt indices present in points
	cap       int
	sessionID string
	durable   bool
}

// NewTracker creates a tracker. When durable is true, checkpoints mirror to
// the on-disk store and previously persisted points rehydrate on construction.
func NewTracker(sessionID string, durable bool) (*Tracker, error) {
	t := &Tracker{
		points:    map[int]*RewindPoint{},
		cap:       defaultCap,
		sessionID: sanitizeSessionID(sessionID),
		durable:   durable,
	}
	if durable {
		if err := t.rehydrate(); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// BeginPrompt opens the rewind window for promptIndex. Calling Begin while a
// window is open closes it first.
func (t *Tracker) BeginPrompt(promptIndex int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_ = t.endLocked()
	t.current = &RewindPoint{
		PromptIndex: promptIndex,
		Before:      map[string]string{},
		After:       map[string]string{},
	}
}

// SetTouchResult records both sides of a touch explicitly: before is kept
// first-wins, after is stored last-wins. This variant avoids double reads.
func (t *Tracker) SetTouchResult(path, beforeContent, afterContent string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return
	}
	if _, seen := t.current.Before[path]; !seen {
		t.current.Before[path] = beforeContent
	}
	t.current.After[path] = afterContent
}

// EndPrompt closes the current window and persists the rewind point.
func (t *Tracker) EndPrompt() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.endLocked()
}

func (t *Tracker) endLocked() error {
	if t.current == nil {
		return nil
	}
	p := t.current
	t.current = nil
	if len(p.Before) == 0 && len(p.After) == 0 {
		return nil // nothing touched: no point worth storing
	}
	if _, exists := t.points[p.PromptIndex]; !exists {
		t.order = append(t.order, p.PromptIndex)
		sort.Ints(t.order)
	}
	t.points[p.PromptIndex] = p
	if t.durable && t.sessionID != "" {
		if err := t.persist(p); err != nil {
			return fmt.Errorf("filestate: persist checkpoint %d: %w", p.PromptIndex, err)
		}
	}
	t.evictLocked()
	return nil
}

// RewindTo returns the restore plan for promptIndex: files whose pre-prompt
// content differs from what is currently on disk, mapped to their restored
// content. It truncates all rewind points at or after promptIndex (a rewind
// discards later history). The caller performs the actual writes.
func (t *Tracker) RewindTo(promptIndex int) (map[string]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	point, ok := t.points[promptIndex]
	if !ok {
		return nil, ErrNoRewindPoint
	}
	plan := map[string]string{}
	for path, before := range point.Before {
		cur, err := os.ReadFile(path) // #nosec G304 -- recorded workspace path
		if err != nil || string(cur) != before {
			plan[path] = before
		}
	}
	// Discard this point and everything after it.
	for idx := range t.points {
		if idx >= promptIndex {
			delete(t.points, idx)
		}
	}
	filtered := t.order[:0]
	for _, idx := range t.order {
		if _, keep := t.points[idx]; keep {
			filtered = append(filtered, idx)
		}
	}
	t.order = filtered
	if t.durable && t.sessionID != "" {
		for idx := range t.pointsOnDisk(promptIndex) {
			_ = os.Remove(filepath.Join(t.storeDir(), fmt.Sprintf(checkpointFormat, idx)))
		}
	}
	return plan, nil
}

// Len reports how many rewind points are held.
func (t *Tracker) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.points)
}

// evictLocked drops the oldest points beyond capacity (memory and disk).
func (t *Tracker) evictLocked() {
	for len(t.order) > t.cap {
		oldest := t.order[0]
		t.order = t.order[1:]
		delete(t.points, oldest)
		if t.durable && t.sessionID != "" {
			_ = os.Remove(filepath.Join(t.storeDir(), fmt.Sprintf(checkpointFormat, oldest)))
		}
	}
}

// storeDir is the durable mirror location inside the user state tree.
func (t *Tracker) storeDir() string {
	return filepath.Join(stateRoot(), "rewind-checkpoints", t.sessionID)
}

func (t *Tracker) persist(p *RewindPoint) error {
	dir := t.storeDir()
	if err := os.MkdirAll(dir, storeDirPerms); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	final := filepath.Join(dir, fmt.Sprintf(checkpointFormat, p.PromptIndex))
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G304 -- sanitized fixed layout
		return err
	}
	return os.Rename(tmp, final) // atomic swap: crash-safe checkpoint
}

// rehydrate loads persisted checkpoints back into memory.
func (t *Tracker) rehydrate() error {
	if t.sessionID == "" {
		return nil
	}
	dir := t.storeDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		var idx int
		if _, err := fmt.Sscanf(e.Name(), checkpointFormat, &idx); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- sanitized dir listing entry
		if err != nil {
			continue
		}
		var p RewindPoint
		if json.Unmarshal(raw, &p) != nil || p.PromptIndex != idx {
			continue
		}
		if p.Before == nil {
			p.Before = map[string]string{}
		}
		if p.After == nil {
			p.After = map[string]string{}
		}
		if _, exists := t.points[idx]; !exists {
			t.order = append(t.order, idx)
			t.points[idx] = &p
		}
	}
	sort.Ints(t.order)
	return nil
}

// pointsOnDisk lists persisted checkpoint indexes >= from (for cleanup).
func (t *Tracker) pointsOnDisk(from int) map[int]bool {
	out := map[int]bool{}
	entries, err := os.ReadDir(t.storeDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		var idx int
		if _, err := fmt.Sscanf(e.Name(), checkpointFormat, &idx); err != nil {
			continue
		}
		if idx >= from {
			out[idx] = true
		}
	}
	return out
}

// sanitizeSessionID strips anything but alphanumerics and appends a short
// hash, preventing path traversal from hostile session IDs (grok-build does
// the same for its rewind stores).
func sanitizeSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	clean := b.String()
	cut := len(clean)
	if cut > 24 {
		cut = 24
	}
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%s-%s", clean[:cut], hex.EncodeToString(sum[:4]))
}

// stateRoot mirrors storage.StateDir without importing it (leaf-package rule:
// filestate stays independent of graycode storage layout choices).
func stateRoot() string {
	if v := strings.TrimSpace(os.Getenv("GRAYCODE_STATE_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "graycode", "state")
	}
	return filepath.Join(home, ".graycode", "state")
}
