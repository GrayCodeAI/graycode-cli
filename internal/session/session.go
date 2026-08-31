package session

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	contracts "github.com/GrayCodeAI/eagle/tools"
	"github.com/GrayCodeAI/hawk/internal/eventlog"
	"github.com/GrayCodeAI/hawk/internal/eventlog/zstdz"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// Message is a persisted conversation message.
type Message struct {
	Role         string                 `json:"role"`
	Content      string                 `json:"content,omitempty"`
	Thinking     string                 `json:"thinking,omitempty"`
	ContentParts []types.ContentPart    `json:"content_parts,omitempty"`
	Images       []string               `json:"images,omitempty"`
	ToolUse      []contracts.ToolCall   `json:"tool_use,omitempty"`
	ToolResults  []contracts.ToolResult `json:"tool_results,omitempty"`
}

// SessionFormatVersion is the JSONL schema revision persisted by Save. Version 0 is
// the historical meta + message-lines shape; version 1 additionally writes the
// append-only event spine as event lines after the messages so the durable record
// carries the "model-visible ⟺ logged" facts. Version 0 remains byte-compatible.
const SessionFormatVersion = 1

// ToolCall is an alias to the shared ToolCall type for persistence.
type ToolCall = contracts.ToolCall

// ToolResult is an alias to the shared ToolResult type for persistence.
type ToolResult = contracts.ToolResult

// Session is a persisted conversation.
type Session struct {
	ID              string    `json:"id"`
	ParentSessionID string    `json:"parent_session_id,omitempty"`
	DelegationDepth int       `json:"delegation_depth,omitempty"`
	Model           string    `json:"model"`
	Provider        string    `json:"provider"`
	Agent           string    `json:"agent,omitempty"`
	CWD             string    `json:"cwd,omitempty"`
	Name            string    `json:"name,omitempty"`
	Messages        []Message `json:"messages"`
	// Events carries the append-only event spine for version-1 sessions. Version-0
	// sessions leave it unset and keep the byte-compatible messages-only shape.
	Events    []eventlog.WireEvent `json:"events,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	// Fence is a monotonically increasing writer token (optional). When set, a
	// remote owner must present an equal-or-newer fence to write; it is advisory
	// metadata for single-owner remote sessions and is backward-compatible with
	// sessions that never set it.
	Fence string `json:"fence,omitempty"`
}

// ErrNotFound identifies a missing durable session without conflating it with
// corruption or I/O failures in one of the supported on-disk formats.
var ErrNotFound = errors.New("session not found")

func sessionsDir() string {
	return storage.SessionsDir()
}

func legacyPathFor(id string) string {
	return filepath.Join(sessionsDir(), id+".json")
}

func jsonlPathFor(id string) string {
	return filepath.Join(sessionsDir(), id+".jsonl")
}

// Save persists a session to disk atomically.
// Writes to a temp file first, then renames — a crash at any point
// leaves either the old valid file or the new valid file, never a partial write.
// When zstd compression is enabled, the JSONL body is written as concatenated
// independently-decodable Zstandard frames (one per event batch), ported from
// DSH's session-persistence-jsonl compression layer.
func Save(s *Session) error {
	return saveWithCompression(s, false)
}

// saveWithCompression is the implementation of Save, with an optional zstd
// compression parameter. When compress is true, the event spine is written as
// concatenated independently-decodable Zstandard frames (ported from DSH's
// session-persistence-jsonl/src/zstd.ts).
func saveWithCompression(s *Session, compress bool) error {
	if s == nil {
		return fmt.Errorf("session is required")
	}
	if err := ValidateID(s.ID); err != nil {
		return err
	}
	if s.CWD == "" {
		if cwd, err := os.Getwd(); err == nil {
			if abs, err := filepath.Abs(cwd); err == nil {
				s.CWD = abs
			} else {
				s.CWD = cwd
			}
		}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	s.UpdatedAt = time.Now()

	dir := sessionsDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}

	// Write to temp file, then atomic rename. The temp name is namespaced
	// with getpid() so two processes (or two saves of the same session from
	// different hawk instances) don't clobber each other's temp file.
	target := jsonlPathFor(s.ID)
	tmp := fmt.Sprintf("%s.tmp.%d", target, os.Getpid())

	// 0600: the session JSONL holds full conversation history (private user
	// state, matching the WAL and 0750 session dir). os.Create would leave it
	// group/world-readable (0666 &^ umask).
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- path built from sessionsDir()+session ID, internal session store
	if err != nil {
		return fmt.Errorf("create temp session file: %w", err)
	}

	w := bufio.NewWriter(f)

	// Write session metadata as first line
	meta := map[string]interface{}{
		"type":       "session_meta",
		"id":         s.ID,
		"model":      s.Model,
		"provider":   s.Provider,
		"agent":      s.Agent,
		"cwd":        s.CWD,
		"name":       s.Name,
		"created_at": s.CreatedAt.Format(time.RFC3339),
		"updated_at": s.UpdatedAt.Format(time.RFC3339),
	}
	if s.ParentSessionID != "" {
		meta["parent_session_id"] = s.ParentSessionID
		meta["parent_session"] = s.ParentSessionID
	}
	if s.DelegationDepth > 0 {
		meta["delegation_depth"] = s.DelegationDepth
	}
	if len(s.Events) > 0 {
		meta["format_version"] = SessionFormatVersion
	}
	if s.Fence != "" {
		meta["fence"] = s.Fence
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("marshal session meta: %w", err)
	}
	if _, err := w.Write(metaData); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write session meta: %w", err)
	}
	if err := w.WriteByte('\n'); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write newline: %w", err)
	}

	// Write each message as a JSON line
	for _, msg := range s.Messages {
		msgData, err := json.Marshal(msg)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("marshal message: %w", err)
		}
		if _, err := w.Write(msgData); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("write message: %w", err)
		}
		if err := w.WriteByte('\n'); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("write newline: %w", err)
		}
	}

	// Write events as JSONL lines, applying chunk-packing compression to runs
	// of assistant.chunk events (ported from DeepSeek Harness chunk-rows.ts).
	// Each StorageRecord is either a verbatim event or a packed text-chunks
	// row; DecodeStorageRecord expands rows back on load. Version-0 sessions
	// carry no extra lines.
	records := eventlog.PackChunkRuns(s.Events)

	if compress {
		// DSH parity: write the event spine as a single zstd frame for
		// independently-checkable compression. Meta + messages stay plaintext.
		var buf bytes.Buffer
		for _, rec := range records {
			var evData []byte
			if rec.ChunkRow != nil {
				evData, err = json.Marshal(rec.ChunkRow)
			} else {
				evData, err = json.Marshal(rec.Event)
			}
			if err != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return fmt.Errorf("marshal event: %w", err)
			}
			buf.Write(evData)
			buf.WriteByte('\n')
		}
		compressed, cerr := zstdz.CompressFrame(buf.Bytes())
		if cerr != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("compress events: %w", cerr)
		}
		if _, err := w.Write(compressed); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("write compressed events: %w", err)
		}
	} else {
		for _, rec := range records {
			var evData []byte
			if rec.ChunkRow != nil {
				evData, err = json.Marshal(rec.ChunkRow)
			} else {
				evData, err = json.Marshal(rec.Event)
			}
			if err != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return fmt.Errorf("marshal event: %w", err)
			}
			if _, err := w.Write(evData); err != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return fmt.Errorf("write event: %w", err)
			}
			if err := w.WriteByte('\n'); err != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return fmt.Errorf("write newline: %w", err)
			}
		}
	}

	if err := w.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("flush session: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync session: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close session file: %w", err)
	}

	// Atomic rename: either old file or new file, never partial
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// WAL (Write-Ahead Log) appends messages incrementally for crash recovery.
// Each message is appended immediately — if hawk crashes, the WAL has everything.
type WAL struct {
	mu   sync.Mutex
	f    *os.File
	path string
	id   string
}

// NewWAL creates or opens a write-ahead log for a session.
func NewWAL(sessionID string) (*WAL, error) {
	if err := ValidateID(sessionID); err != nil {
		return nil, err
	}
	dir := sessionsDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionID+".wal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path built from sessionsDir()+session ID, internal session store
	if err != nil {
		return nil, fmt.Errorf("opening WAL: %w", err)
	}

	return &WAL{f: f, path: path, id: sessionID}, nil
}

// Append writes a message to the WAL immediately. This is crash-safe:
// even if the process dies right after, the message is on disk.
func (w *WAL) Append(msg Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if _, err := w.f.Write(data); err != nil {
		return err
	}
	return w.f.Sync()
}

// AppendMeta writes session metadata to the WAL.
func (w *WAL) AppendMeta(model, provider, cwd string) error {
	return w.AppendMetaWithFence(model, provider, cwd, "")
}

// AppendMetaWithFence writes session metadata to the WAL, including an optional
// writer fence token for single-owner remote sessions.
func (w *WAL) AppendMetaWithFence(model, provider, cwd, fence string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	meta := map[string]interface{}{
		"type":       "session_meta",
		"id":         w.id,
		"model":      model,
		"provider":   provider,
		"cwd":        cwd,
		"created_at": time.Now().Format(time.RFC3339),
	}
	if fence != "" {
		meta["fence"] = fence
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	data = append(data, '\n')

	if _, err := w.f.Write(data); err != nil {
		return err
	}
	return w.f.Sync()
}

// Close closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

// Remove deletes the WAL file (called after successful Save).
func (w *WAL) Remove() error {
	_ = w.Close()
	return os.Remove(w.path)
}

// RecoverFromWAL rebuilds a session from a WAL file if one exists.
// Returns nil if no WAL exists.
func RecoverFromWAL(sessionID string) (*Session, error) {
	if err := ValidateID(sessionID); err != nil {
		return nil, err
	}
	path := filepath.Join(sessionsDir(), sessionID+".wal")
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir()+session ID, internal session store
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // no WAL, nothing to recover
		}
		return nil, fmt.Errorf("open recovery WAL %s: %w", sessionID, err)
	}
	defer func() { _ = f.Close() }()

	// Use the same tolerant line scanner as JSONL loading: oversize/corrupt
	// lines no longer brick recovery (LOW finding). The WAL's first record is
	// session_meta, so the meta is captured by scanJSONLLines.
	meta, messages, events, err := scanJSONLLines(f, sessionID)
	if err != nil {
		return nil, fmt.Errorf("recover session %s: %w", sessionID, err)
	}
	if len(messages) == 0 {
		return nil, nil
	}

	var s Session
	s.ID = sessionID
	s.Messages = messages
	if len(events) > 0 {
		decoded, derr := eventlog.DecodeWire(events)
		if derr != nil {
			return nil, fmt.Errorf("recover session %s: %w", sessionID, derr)
		}
		if rerr := eventlog.ValidateRelations(decoded); rerr != nil {
			slog.Warn("session: relational invariant violation (recovery)", "session", sessionID, "err", rerr)
		}
		s.Events = events
	}
	s.Model = asString(meta["model"])
	s.Provider = asString(meta["provider"])
	s.Agent = asString(meta["agent"])
	s.CWD = asString(meta["cwd"])
	if p := asString(meta["parent_session_id"]); p != "" {
		s.ParentSessionID = p
	} else if p := asString(meta["parent_session"]); p != "" {
		s.ParentSessionID = p
	}
	if d, ok := meta["delegation_depth"].(float64); ok {
		s.DelegationDepth = int(d)
	} else if d, ok := meta["delegation_depth"].(int); ok {
		s.DelegationDepth = d
	}
	if v, ok := meta["created_at"].(string); ok {
		s.CreatedAt, _ = time.Parse(time.RFC3339, v)
	}
	s.Fence = asString(meta["fence"])
	s.UpdatedAt = time.Now()
	return &s, nil
}

// CheckForRecovery looks for any WAL files and offers recovery.
// Returns session IDs that have WAL files.
func CheckForRecovery() []string {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var ids []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".wal" {
			id := e.Name()[:len(e.Name())-4]
			if ValidID(id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// Load reads a session from disk, supporting both JSONL and legacy JSON formats.
func Load(id string) (*Session, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	// Try JSONL first
	s, jsonlErr := loadJSONL(id)
	if jsonlErr == nil {
		return s, nil
	}
	s, legacyErr := loadLegacyJSON(id)
	if legacyErr == nil {
		return s, nil
	}
	if errors.Is(jsonlErr, os.ErrNotExist) && errors.Is(legacyErr, os.ErrNotExist) {
		return nil, fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	return nil, fmt.Errorf("load session %s: %w", id, errors.Join(jsonlErr, legacyErr))
}

func loadJSONL(id string) (*Session, error) {
	path := jsonlPathFor(id)
	// Check for zstd-compressed variant
	zstdPath := jsonlPathForCompressed(id, true)
	if _, err := os.Stat(zstdPath); err == nil {
		return loadZstdJSONLFile(zstdPath, id)
	}
	return loadJSONLFile(path, id)
}

// scanJSONLLines reads a JSONL session file tolerantly: lines larger than the
// per-line cap are drained+logged and skipped (the prior 1MB bufio.Scanner
// cap bricked the whole load on a single oversized line — LOW finding), and
// JSON-corrupt lines are logged+skipped rather than failing the load. The
// first non-empty line is decoded into meta. Message lines carry a role and
// become Session.Messages; version-1 event lines (type in the eventlog
// vocabulary) become Session.Events.
func scanJSONLLines(r io.Reader, logID string) (meta map[string]any, messages []Message, events []eventlog.WireEvent, err error) {
	const maxLine = 16 * 1024 * 1024
	reader := bufio.NewReaderSize(r, maxLine)
	flushOversize := func() {
		_, _ = reader.ReadString('\n') // drain the remainder of an oversize line
	}
	lineNo := 0
	firstLine := true
	for {
		line, lpErr := reader.ReadSlice('\n')
		isPrefix := errors.Is(lpErr, bufio.ErrBufferFull)
		if isPrefix {
			flushOversize()
			lineNo++
			slog.Warn("session: skipped oversize line", "session", logID, "line", lineNo, "max", maxLine)
			firstLine = false
			continue
		}
		if lpErr != nil && !(errors.Is(lpErr, io.EOF) && len(line) > 0) {
			if errors.Is(lpErr, io.EOF) {
				break
			}
			return nil, nil, nil, lpErr
		}
		lineNo++
		raw := bytes.TrimRight(line, "\r\n")
		if len(bytes.TrimSpace(raw)) == 0 {
			if errors.Is(lpErr, io.EOF) {
				break
			}
			continue
		}
		if firstLine {
			firstLine = false
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err == nil {
				meta = m
			} else {
				// First non-empty line is not valid JSON — the session file is
				// corrupt, not merely empty. Surface this as a load error
				// (distinct from ErrNotFound) so callers report a 500, not 404.
				return nil, nil, nil, fmt.Errorf("session %s: parse meta line %d: %w", logID, lineNo, err)
			}
			if errors.Is(lpErr, io.EOF) {
				break
			}
			continue
		}
		// Version-1 event lines carry a "type" in the eventlog vocabulary.
		// Distinguish them from message lines before falling through to Message.
		var kind struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &kind)
		// A chunk-packing storage row uses a bare tag (text-chunks) that is
		// NOT in the eventlog Type vocabulary. Expand it into individual events.
		if eventlog.IsStorageRecord(kind.Type) {
			decoded, derr := eventlog.DecodeStorageRecord(raw)
			if derr != nil {
				slog.Warn("session: skipped corrupted chunk row", "session", logID, "line", lineNo, "err", derr)
			} else {
				events = append(events, decoded...)
			}
			if errors.Is(lpErr, io.EOF) {
				break
			}
			continue
		}
		// Version-1 event lines carry a "type" in the eventlog vocabulary.
		// Distinguish them from message lines before falling through to Message.
		if eventlog.Type(kind.Type).Known() {
			var ev eventlog.WireEvent
			if jerr := json.Unmarshal(raw, &ev); jerr != nil {
				slog.Warn("session: skipped corrupted event line", "session", logID, "line", lineNo, "err", jerr)
			} else {
				events = append(events, ev)
			}
			if errors.Is(lpErr, io.EOF) {
				break
			}
			continue
		}
		var msg Message
		if jerr := json.Unmarshal(raw, &msg); jerr != nil {
			slog.Warn("session: skipped corrupted line", "session", logID, "line", lineNo, "err", jerr)
			if errors.Is(lpErr, io.EOF) {
				break
			}
			continue
		}
		messages = append(messages, msg)
		if errors.Is(lpErr, io.EOF) {
			break
		}
	}
	return meta, messages, events, nil
}

func loadJSONLFile(path, id string) (*Session, error) {
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir()+session ID, internal session store
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	meta, messages, events, err := scanJSONLLines(f, id)
	if err != nil {
		return nil, fmt.Errorf("read session %s: %w", id, err)
	}
	var s Session
	s.ID = id
	s.Messages = messages
	// Fail loud on a version-1 event spine that does not validate, so a record
	// the build cannot project is never trusted or silently rewritten.
	if len(events) > 0 {
		decoded, derr := eventlog.DecodeWire(events)
		if derr != nil {
			return nil, fmt.Errorf("read session %s: %w", id, derr)
		}
		// Relational validation is best-effort: structural validity is the
		// hard gate (DecodeWire above); relational violations are warnings
		// so legacy logs with incomplete turn/step metadata still load.
		if rerr := eventlog.ValidateRelations(decoded); rerr != nil {
			slog.Warn("session: relational invariant violation", "session", id, "err", rerr)
		}
		s.Events = events
	}
	if meta != nil {
		s.Model = asString(meta["model"])
		s.Provider = asString(meta["provider"])
		s.Agent = asString(meta["agent"])
		s.CWD = asString(meta["cwd"])
		s.Name = asString(meta["name"])
		if p := asString(meta["parent_session_id"]); p != "" {
			s.ParentSessionID = p
		} else if p := asString(meta["parent_session"]); p != "" {
			s.ParentSessionID = p
		}
		if d, ok := meta["delegation_depth"].(float64); ok {
			s.DelegationDepth = int(d)
		} else if d, ok := meta["delegation_depth"].(int); ok {
			s.DelegationDepth = d
		}
		if v, ok := meta["created_at"].(string); ok {
			s.CreatedAt, _ = time.Parse(time.RFC3339, v)
		}
		if v, ok := meta["updated_at"].(string); ok {
			s.UpdatedAt, _ = time.Parse(time.RFC3339, v)
		}
		s.Fence = asString(meta["fence"])
	}
	if len(s.Messages) == 0 && meta == nil {
		return nil, ErrNotFound
	}
	return &s, nil
}

// asString safely extracts a string value from a JSON-decoded map.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func loadLegacyJSON(id string) (*Session, error) {
	return loadLegacyJSONFile(legacyPathFor(id))
}

func loadLegacyJSONFile(path string) (*Session, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path built from sessionsDir()+session ID, internal session store
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Entry is a summary of a saved session for listing.
type Entry struct {
	ID        string
	Preview   string
	CWD       string
	UpdatedAt time.Time
}

// List returns all saved sessions, newest first.
// Uses file modification time for sorting to avoid loading all sessions.
func List() ([]Entry, error) {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Entry
	for _, e := range entries {
		name := e.Name()
		// Handle compound extension .jsonl.zstd
		id := name
		switch {
		case strings.HasSuffix(name, ".jsonl.zstd"):
			id = name[:len(name)-len(".jsonl.zstd")]
		case strings.HasSuffix(name, ".jsonl"):
			id = name[:len(name)-len(".jsonl")]
		case strings.HasSuffix(name, ".json"):
			id = name[:len(name)-len(".json")]
		default:
			continue
		}

		// Use file info for timestamp (fast, no parsing needed)
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Only load the first user message for preview (don't parse full file)
		preview := loadPreview(filepath.Join(dir, e.Name()))
		out = append(out, Entry{
			ID:        id,
			Preview:   preview,
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// loadPreview reads only enough of a session file to extract the first user message.
func loadPreview(path string) string {
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir() + directory entry name returned by os.ReadDir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 4096) // small buffer, just need first few lines
	linesRead := 0

	for scanner.Scan() && linesRead < 10 {
		linesRead++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg Message
		if json.Unmarshal(line, &msg) == nil && msg.Role == "user" && msg.Content != "" {
			preview := msg.Content
			if runes := []rune(preview); len(runes) > 80 {
				preview = string(runes[:80]) + "..."
			}
			return preview
		}
	}
	return ""
}

// LoadLatestForCWD returns the newest saved session for cwd.
func LoadLatestForCWD(cwd string) (*Session, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	// Scan files by modification time without loading all sessions
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		id   string
		time time.Time
	}
	var candidates []candidate

	for _, e := range entries {
		name := e.Name()
		var id string
		if strings.HasSuffix(name, ".jsonl.zstd") {
			id = name[:len(name)-len(".jsonl.zstd")]
		} else {
			ext := filepath.Ext(name)
			if ext != ".jsonl" && ext != ".json" {
				continue
			}
			id = name[:len(name)-len(ext)]
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{id: id, time: info.ModTime()})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].time.After(candidates[j].time)
	})

	// Check most recent sessions until we find one matching CWD
	for _, c := range candidates {
		s, err := Load(c.id)
		if err != nil {
			continue
		}
		if s.CWD == cwd || s.CWD == "" {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no saved session for %s", cwd)
}

// LoadLatest returns the newest saved session regardless of CWD.
func LoadLatest() (*Session, error) {
	entries, err := List()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no saved sessions")
	}
	return Load(entries[0].ID)
}

// MigrateToJSONL converts a legacy JSON session to JSONL format.
func MigrateToJSONL(id string) error {
	s, err := loadLegacyJSON(id)
	if err != nil {
		return err
	}
	return Save(s)
}

// JsonlCompression selects the physical encoding for JSONL session artifacts.
// Ported from DSH's session-persistence-jsonl/src/format.ts.
type JsonlCompression string

const (
	// JsonlCompressionNone writes plaintext JSONL lines.
	JsonlCompressionNone JsonlCompression = "none"
	// JsonlCompressionZstd writes the event spine as concatenated independent
	// Zstandard frames while leaving meta + message lines plaintext.
	JsonlCompressionZstd JsonlCompression = "zstd"
)

// jsonlPathFor returns the JSONL artifact path for a session with optional
// zstd compression suffix, matching DSH's sessionDirectory/logPath layout.
func jsonlPathForCompressed(id string, compress bool) string {
	var suffix string
	if compress {
		suffix = logSuffix(JsonlCompressionZstd)
	} else {
		suffix = logSuffix(JsonlCompressionNone)
	}
	return filepath.Join(sessionsDir(), id+suffix)
}

// logSuffix returns the file suffix for one compression encoding.
// Ported from DSH's logSuffix().
func logSuffix(c JsonlCompression) string {
	if c == JsonlCompressionZstd {
		return ".jsonl.zstd"
	}
	return ".jsonl"
}

// SaveWithZstd persists a session with zstd frame compression on the event
// spine, matching DSH's session-persistence-jsonl compression layer.
func SaveWithZstd(s *Session) error {
	return saveWithCompression(s, true)
}

// parseHeaderMeta reads only the first line of a session file and returns the
// parsed session metadata, without parsing the event spine or messages.
// Ported from DSH's parseHeaderMeta — used by List to avoid loading every
// session's full history just for session listing.
func parseHeaderMeta(path string) (map[string]any, error) {
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir() + directory entry name from os.ReadDir
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// For zstd-compressed files, we need to check the magic bytes of subsequent
	// data; but the header is always plaintext as the first line.
	reader := bufio.NewReaderSize(f, 65536)
	line, err := reader.ReadSlice('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
		return nil, err
	}
	raw := bytes.TrimRight(line, "\r\n")
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// detectCompression checks whether a session file uses zstd compression by
// examining the file extension (.jsonl.zstd) or zstd magic bytes. Returns
// JsonlCompressionNone for plaintext files.
func detectCompression(path string) JsonlCompression {
	if strings.HasSuffix(path, ".jsonl.zstd") {
		return JsonlCompressionZstd
	}
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir()+session ID
	if err != nil {
		return JsonlCompressionNone
	}
	defer func() { _ = f.Close() }()

	reader := bufio.NewReaderSize(f, 65536)
	// Skip all plaintext JSONL lines (header + messages) until we hit binary data
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return JsonlCompressionNone
		}
		// Check if this line starts with a zstd magic byte (binary frame)
		if len(line) >= 4 && binary.LittleEndian.Uint32([]byte(line[:4])) == zstdz.ZstdMagic {
			return JsonlCompressionZstd
		}
		// If the line is not valid JSON (doesn't start with '{'), it's binary
		trimmed := bytes.TrimSpace([]byte(line))
		if len(trimmed) == 0 {
			return JsonlCompressionNone
		}
		if trimmed[0] != '{' {
			// Not JSON, likely binary zstd data without full magic on this read
			return JsonlCompressionNone
		}
		// Line is JSON — continue to next line
	}
}

// loadZstdJSONLFile reads a zstd-compressed JSONL session file. The header
// line and message lines are plaintext; the event spine is written as
// concatenated independently-decodable Zstandard frames.
func loadZstdJSONLFile(path, id string) (*Session, error) {
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir()+session ID
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Use SessionLogScanner to read header + messages (plaintext), then
	// decompress zstd frames for events.
	scanner := NewSessionLogScanner(f, id)
	if err := scanner.Scan(); err != nil {
		return nil, fmt.Errorf("read zstd session %s: %w", id, err)
	}
	meta := scanner.Header()
	messages := scanner.Messages()

	// Read remaining bytes and decompress zstd frames
	remaining := scanner.Remaining()
	if len(remaining) > 0 {
		scan := zstdz.ScanFrames(remaining, 0)
		dec := zstdz.NewFrameDecoder()
		defer dec.Close()
		for frameData := range dec.Decode(remaining, scan.Frames) {
			for _, raw := range bytes.Split(bytes.TrimRight(frameData, "\n"), []byte("\n")) {
				if len(bytes.TrimSpace(raw)) == 0 {
					continue
				}
				// Decode as chunk row or event line
				var kind struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(raw, &kind)
				if eventlog.IsStorageRecord(kind.Type) {
					decoded, derr := eventlog.DecodeStorageRecord(raw)
					if derr != nil {
						slog.Warn("session: skipped corrupted chunk row in zstd", "session", id, "err", derr)
						continue
					}
					scanner.events = append(scanner.events, decoded...)
					continue
				}
				if eventlog.Type(kind.Type).Known() {
					var ev eventlog.WireEvent
					if jerr := json.Unmarshal(raw, &ev); jerr != nil {
						slog.Warn("session: skipped corrupted event in zstd", "session", id, "err", jerr)
						continue
					}
					scanner.events = append(scanner.events, ev)
				}
			}
		}
	}

	var s Session
	s.ID = id
	s.Messages = messages
	events := scanner.Events()
	if len(events) > 0 {
		decoded, derr := eventlog.DecodeWire(events)
		if derr != nil {
			return nil, fmt.Errorf("read zstd session %s: %w", id, derr)
		}
		// Relational validation is best-effort for the same reason as loadJSONLFile.
		if rerr := eventlog.ValidateRelations(decoded); rerr != nil {
			slog.Warn("session: relational invariant violation (zstd)", "session", id, "err", rerr)
		}
		s.Events = events
	}
	if meta != nil {
		s.Model = asString(meta["model"])
		s.Provider = asString(meta["provider"])
		s.Agent = asString(meta["agent"])
		s.CWD = asString(meta["cwd"])
		s.Name = asString(meta["name"])
		if v, ok := meta["created_at"].(string); ok {
			s.CreatedAt, _ = time.Parse(time.RFC3339, v)
		}
		if v, ok := meta["updated_at"].(string); ok {
			s.UpdatedAt, _ = time.Parse(time.RFC3339, v)
		}
	}
	if len(s.Messages) == 0 && meta == nil {
		return nil, ErrNotFound
	}
	return &s, nil
}

// SessionLogScanResult is the structured result of scanning a session log.
// Ported from DSH's SessionLogScan interface.
type SessionLogScanResult struct {
	Meta           map[string]any
	Messages       []Message
	Events         []eventlog.WireEvent
	CommittedBytes int64 // safe truncation offset
	Error          error
}

// SessionLogScanner is an incremental JSONL session log scanner with DSH parity:
// it validates sequence ordering, tracks committed bytes, and handles torn
// final records. Ported from DSH's session-persistence-jsonl/src/format.ts
// SessionLogScanner class.
type SessionLogScanner struct {
	reader         *bufio.Reader
	meta           map[string]any
	messages       []Message
	events         []eventlog.WireEvent
	committedBytes int64
	inputBytes     int64
	issue          error
	finished       bool
	tornStart      bool
}

// NewSessionLogScanner creates a scanner that reads header + messages from
// a JSONL session file (or a reader over one).
func NewSessionLogScanner(r io.Reader, logID string) *SessionLogScanner {
	return &SessionLogScanner{
		reader: bufio.NewReaderSize(r, 16*1024*1024),
	}
}

// Header returns the parsed session metadata from the first line.
func (s *SessionLogScanner) Header() map[string]any { return s.meta }

// Messages returns the parsed messages (non-event lines).
func (s *SessionLogScanner) Messages() []Message { return s.messages }

// Events returns the parsed events (version-1 event lines).
func (s *SessionLogScanner) Events() []eventlog.WireEvent { return s.events }

// CommittedBytes returns the byte offset safe to append at (last complete record).
func (s *SessionLogScanner) CommittedBytes() int64 { return s.committedBytes }

// Error returns any corruption error encountered during scanning.
func (s *SessionLogScanner) Error() error { return s.issue }

// Remaining reads and returns all bytes after the scanner's position.
func (s *SessionLogScanner) Remaining() []byte {
	data, _ := io.ReadAll(s.reader)
	return data
}

// Scan processes the next available data from the file. It reads the header
// line, then all message lines and event lines until EOF or corruption.
// Sequence ordering is validated: event seqs must be contiguous from 0.
func (s *SessionLogScanner) Scan() error {
	if s.finished {
		return s.issue
	}
	lineNo := 0
	firstLine := true
	for {
		line, lpErr := s.reader.ReadSlice('\n')
		isPrefix := errors.Is(lpErr, bufio.ErrBufferFull)
		if isPrefix {
			// Drain oversize line
			_, _ = s.reader.ReadString('\n')
			slog.Warn("session: skipped oversize line", "line", lineNo, "max", 16*1024*1024)
			s.inputBytes += int64(len(line))
			lineNo++
			firstLine = false
			continue
		}
		// Check for zstd frame magic after the first line
		if firstLine && len(line) > 0 {
			// Check if this is a zstd frame (not a JSON header line)
			if len(line) >= 4 && binary.LittleEndian.Uint32(line[:4]) == zstdz.ZstdMagic {
				s.tornStart = true
				s.finished = true
				return s.issue
			}
		}
		s.inputBytes += int64(len(line))
		if lpErr != nil && !(errors.Is(lpErr, io.EOF) && len(line) > 0) {
			if errors.Is(lpErr, io.EOF) {
				s.finished = true
				return s.issue
			}
			s.issue = lpErr
			s.finished = true
			return s.issue
		}
		raw := bytes.TrimRight(line, "\r\n")
		if len(bytes.TrimSpace(raw)) == 0 {
			if errors.Is(lpErr, io.EOF) {
				s.finished = true
				return s.issue
			}
			continue
		}
		if firstLine {
			firstLine = false
			if err := json.Unmarshal(raw, &s.meta); err != nil {
				s.issue = fmt.Errorf("parse meta line %d: %w", lineNo, err)
				s.finished = true
				return s.issue
			}
			s.committedBytes = s.inputBytes
			if errors.Is(lpErr, io.EOF) {
				s.finished = true
				return s.issue
			}
			continue
		}
		lineNo++
		s.processEventOrMessage(raw, lineNo)
		s.committedBytes = s.inputBytes // update committed boundary
		if errors.Is(lpErr, io.EOF) {
			s.finished = true
			return s.issue
		}
	}
}

// processEventOrMessage classifies a JSONL line as event, chunk row, or message.
// Validates event sequence ordering (must be contiguous from 0).
func (s *SessionLogScanner) processEventOrMessage(raw []byte, lineNo int) {
	var kind struct {
		Type string `json:"type"`
		Seq  uint64 `json:"seq"`
	}
	_ = json.Unmarshal(raw, &kind)

	if eventlog.IsStorageRecord(kind.Type) {
		decoded, derr := eventlog.DecodeStorageRecord(raw)
		if derr != nil {
			slog.Warn("session: skipped corrupted chunk row", "line", lineNo, "err", derr)
			return
		}
		s.events = append(s.events, decoded...)
		return
	}

	if eventlog.Type(kind.Type).Known() {
		var ev eventlog.WireEvent
		if jerr := json.Unmarshal(raw, &ev); jerr != nil {
			slog.Warn("session: skipped corrupted event line", "line", lineNo, "err", jerr)
			return
		}
		// Validate sequence ordering (DSH SessionLogScanner invariant)
		expectedSeq := uint64(len(s.events))
		if ev.Seq != expectedSeq {
			s.issue = fmt.Errorf("seq gap at line %d: expected %d, got %d", lineNo, expectedSeq, ev.Seq)
			return
		}
		s.events = append(s.events, ev)
		return
	}

	var msg Message
	if jerr := json.Unmarshal(raw, &msg); jerr != nil {
		slog.Warn("session: skipped corrupted line", "line", lineNo, "err", jerr)
		return
	}
	s.messages = append(s.messages, msg)
}
