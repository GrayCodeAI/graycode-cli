// Package securitylog implements a tamper-evident, append-only security
// event log. Every entry is chained to the previous one with HMAC-SHA256 so
// that reordering, deletion, or alteration of any historical event is
// detectable during verification.
package securitylog

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// EventSeverity classifies the impact of a security event.
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityCritical EventSeverity = "critical"
)

// Event describes a single security event. Hash and PrevHash are the chain
// linkage and are computed at write time; callers supply the rest.
type Event struct {
	Seq       uint64        `json:"seq"`
	Timestamp time.Time     `json:"timestamp"`
	Severity  EventSeverity `json:"severity"`
	Type      string        `json:"type"`
	Detail    string        `json:"detail,omitempty"`
	Tool      string        `json:"tool,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	PrevHash  string        `json:"prev_hash"`
	Hash      string        `json:"hash"`
}

const (
	keyFileName  = "sel.key"
	logFileName  = "security_events.jsonl"
	headFileName = "sel.head"
	keySize      = 32
	genesisHash  = "" // the first entry links against the empty string
)

// headPointer is a separate file recording the expected tail of the chain.
// It makes truncation detectable: a removed tail no longer matches the head.
type headPointer struct {
	Seq  uint64 `json:"seq"`
	Hash string `json:"hash"`
}

// Log is the append-only hash-chained security event log.
type Log struct {
	mu      sync.Mutex
	dir     string
	path    string
	keyPath string
	key     []byte
	f       *os.File
	seq     uint64
	last    string
	closed  bool
}

// DefaultDir returns the default on-disk location for the security event log,
// rooted under graycode's per-user state directory.
func DefaultDir() string {
	return filepath.Join(storage.StateDir(), "securitylog")
}

// New opens (or creates) a security event log rooted at dir. The HMAC key is
// generated once on first use and reused for subsequent opens, so verification
// is stable across processes.
func New(dir string) (*Log, error) {
	if dir == "" {
		return nil, fmt.Errorf("securitylog: empty directory")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("securitylog: create dir: %w", err)
	}

	l := &Log{
		dir:     dir,
		path:    filepath.Join(dir, logFileName),
		keyPath: filepath.Join(dir, keyFileName),
	}

	if err := l.loadKey(); err != nil {
		return nil, err
	}
	if err := l.loadChain(); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path is derived from the state dir, not external input
	if err != nil {
		return nil, fmt.Errorf("securitylog: open log: %w", err)
	}
	l.f = f
	return l, nil
}

// loadKey reads an existing HMAC key or generates a fresh one.
func (l *Log) loadKey() error {
	data, err := os.ReadFile(l.keyPath)
	if err == nil && len(data) == keySize {
		l.key = data
		return nil
	}
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("securitylog: generate key: %w", err)
	}
	if err := os.WriteFile(l.keyPath, key, 0o600); err != nil {
		return fmt.Errorf("securitylog: persist key: %w", err)
	}
	l.key = key
	return nil
}

// loadChain scans the existing log to recover the current sequence number and
// tail hash so appends continue the chain without gaps. It also reads the head
// pointer so a truncated tail is caught at append time, not silently extended.
func (l *Log) loadChain() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("securitylog: read log: %w", err)
	}
	var ev Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("securitylog: corrupt entry: %w", err)
		}
		l.seq = ev.Seq
		l.last = ev.Hash
	}

	if head, err := l.readHead(); err == nil && head.Seq != 0 {
		if head.Seq != l.seq || head.Hash != l.last {
			return fmt.Errorf("securitylog: log tail does not match head pointer (truncated or tampered)")
		}
	}
	return nil
}

func (l *Log) readHead() (headPointer, error) {
	var head headPointer
	data, err := os.ReadFile(filepath.Join(l.dir, headFileName))
	if err != nil {
		return head, err
	}
	err = json.Unmarshal(data, &head)
	return head, err
}

// writeHead persists the current chain tail so truncation is detectable.
func (l *Log) writeHead() error {
	head := headPointer{Seq: l.seq, Hash: l.last}
	data, err := json.Marshal(head)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.dir, headFileName), data, 0o600)
}

// Append records a new event and returns the linked entry. It is safe for
// concurrent use. The returned event carries the computed chain linkage.
func (l *Log) Append(severity EventSeverity, eventType, detail, tool, sessionID string) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return Event{}, fmt.Errorf("securitylog: log closed")
	}

	l.seq++
	ev := Event{
		Seq:       l.seq,
		Timestamp: time.Now().UTC(),
		Severity:  severity,
		Type:      eventType,
		Detail:    detail,
		Tool:      tool,
		SessionID: sessionID,
		PrevHash:  l.last,
	}
	ev.Hash = l.computeHash(ev)

	line, err := json.Marshal(ev)
	if err != nil {
		return Event{}, fmt.Errorf("securitylog: marshal: %w", err)
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		return Event{}, fmt.Errorf("securitylog: append: %w", err)
	}
	l.last = ev.Hash
	if err := l.writeHead(); err != nil {
		return Event{}, fmt.Errorf("securitylog: write head: %w", err)
	}
	return ev, nil
}

// computeHash HMACs the canonical serialization of the event with the chain
// key. The hash covers every field except Hash itself.
func (l *Log) computeHash(ev Event) string {
	ev.Hash = ""
	payload, _ := json.Marshal(ev)
	mac := hmac.New(sha256.New, l.key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify replays the log and confirms every entry's hash matches its contents
// and each entry chains to the previous one. It returns the number of entries
// verified and an error naming the first break in the chain.
func Verify(dir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		if os.IsNotExist(err) {
			// No log to verify yet is vacuously intact.
			return 0, nil
		}
		return 0, fmt.Errorf("securitylog: read log: %w", err)
	}
	key, err := os.ReadFile(filepath.Join(dir, keyFileName))
	if err != nil {
		return 0, fmt.Errorf("securitylog: read key: %w", err)
	}
	if len(key) != keySize {
		return 0, fmt.Errorf("securitylog: invalid key size %d", len(key))
	}

	count := 0
	prev := genesisHash
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return count, fmt.Errorf("securitylog: entry %d corrupt: %w", count, err)
		}
		if ev.PrevHash != prev {
			return count, fmt.Errorf("securitylog: entry %d breaks chain: expected prev %q got %q", count, prev, ev.PrevHash)
		}
		computed := hashEntry(key, ev)
		if computed != ev.Hash {
			return count, fmt.Errorf("securitylog: entry %d hash mismatch", count)
		}
		prev = ev.Hash
		count++
	}

	// Confirm the chain tail matches the recorded head pointer, catching
	// truncation of the tail.
	var head headPointer
	if data, err := os.ReadFile(filepath.Join(dir, headFileName)); err == nil {
		_ = json.Unmarshal(data, &head)
	}
	if head.Seq != 0 {
		if uint64(count) != head.Seq || prev != head.Hash {
			return count, fmt.Errorf("securitylog: tail does not match head pointer (truncated)")
		}
	}
	return count, nil
}

// hashEntry recomputes an event's chain hash without mutating it.
func hashEntry(key []byte, ev Event) string {
	ev.Hash = ""
	payload, _ := json.Marshal(ev)
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Entries reads and decodes every event in the log, oldest first. It does not
// verify the chain (see Verify); it exists for inspection and display.
func Entries(dir string) ([]Event, error) {
	data, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("securitylog: read log: %w", err)
	}
	var events []Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return events, fmt.Errorf("securitylog: corrupt entry: %w", err)
		}
		events = append(events, ev)
	}
	return events, nil
}

// Close flushes and closes the underlying file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.f != nil {
		return l.f.Close()
	}
	return nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
