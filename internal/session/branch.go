package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
)

// Fork creates a new session branched from the given session at the given
// message index. All messages up to and including atIndex are copied to the
// new session. If the session has a version-1 event spine, events up to the
// corresponding sequence are also forked so the child session is fully
// resumable from its own append-only log.
func Fork(sessionID string, atIndex int) (*Session, error) {
	original, err := Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session for fork: %w", err)
	}

	if atIndex < 0 || atIndex >= len(original.Messages) {
		return nil, fmt.Errorf("invalid fork index %d (session %s has %d messages)", atIndex, sessionID, len(original.Messages))
	}

	newID := generateForkID()

	// Determine the event sequence cutoff: map the message index to an event
	// sequence. We use the SessionEndSeed boundary if present; otherwise we
	// fork all events (the child gets a fresh seed boundary in NewSessionWithClient).
	events := forkEventsAt(original.Events, atIndex)

	forked := &Session{
		ID:        newID,
		Model:     original.Model,
		Provider:  original.Provider,
		CWD:       original.CWD,
		Name:      fmt.Sprintf("fork of %s at %d", sessionID, atIndex),
		Messages:  make([]Message, atIndex+1),
		Events:    events,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	copy(forked.Messages, original.Messages[:atIndex+1])

	if err := Save(forked); err != nil {
		return nil, fmt.Errorf("save forked session: %w", err)
	}

	return forked, nil
}

// ForkAtEvent creates a new session branched from a specific event sequence
// number in the parent's event spine. This is the DSH repair/fork seam: it
// truncates the event spine at seqCutoff and the message list at the point
// that corresponds to the last user/assistant message before that sequence.
//
// If the parent has no event spine (version-0 session), falls back to Fork
// at the message index derived from seqCutoff.
func ForkAtEvent(sessionID string, seqCutoff uint64) (*Session, error) {
	original, err := Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session for event-fork: %w", err)
	}

	newID := generateForkID()

	// Truncate the event spine at the requested sequence boundary.
	events := truncateEventsAt(original.Events, seqCutoff)

	// Derive the message cutoff from the event spine: find the last
	// UserMessage/AssistantMsg event at or before seqCutoff and map its
	// order to a message index. If we can't correlate (version-0 or
	// events lack message ordering), fall back to all messages.
	msgCutoff := deriveMessageIndex(events)
	if msgCutoff < 0 {
		// No message events in the truncated spine — copy all messages.
		msgCutoff = len(original.Messages) - 1
	}
	if msgCutoff >= len(original.Messages) {
		msgCutoff = len(original.Messages) - 1
	}

	forked := &Session{
		ID:        newID,
		Model:     original.Model,
		Provider:  original.Provider,
		CWD:       original.CWD,
		Name:      fmt.Sprintf("fork of %s at event seq %d", sessionID, seqCutoff),
		Messages:  make([]Message, msgCutoff+1),
		Events:    events,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	copy(forked.Messages, original.Messages[:msgCutoff+1])

	// Emit the session end-seed so the child fork gets a seed boundary:
	// events before the marker came from the parent fork; events after
	// are live child events.
	if forked.Events != nil {
		forked.Events = append(forked.Events, eventlog.WireEvent{
			Type: eventlog.SessionEndSeed,
			Seq:  seqCutoff,
			At:   time.Now(),
		})
	}

	if err := Save(forked); err != nil {
		return nil, fmt.Errorf("save forked session: %w", err)
	}

	return forked, nil
}

// RepairJournal validates the event spine in a saved session and, if corrupted,
// truncates at the last valid event. This is the DSH repair.ts seam: crash
// recovery that truncates to the last consistent state. It reads the raw JSONL
// file directly (bypassing Load's validation that would reject corrupt spines),
// finds the longest valid prefix, truncates, and rewrites the file atomically.
// Returns the number of events pruned (0 = no repair needed).
func RepairJournal(sessionID string) (int, error) {
	path := jsonlPathFor(sessionID)
	f, err := os.Open(path) // #nosec G304 -- path built from sessionsDir()+session ID
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("repair session: open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Read raw events without validation (scanJSONLLines already tolerates
	// per-line corruption by skipping bad lines, but unknown types or
	// invalid payloads pass through to the events slice).
	meta, _, events, err := scanJSONLLines(f, sessionID)
	if err != nil {
		return 0, fmt.Errorf("repair session: scan: %w", err)
	}

	if len(events) == 0 {
		return 0, nil // nothing to repair
	}

	// If the spine validates, no repair needed.
	if _, err := eventlog.DecodeWire(events); err == nil {
		return 0, nil
	}

	// Find the last valid prefix; DecodeWire validates the entire slice,
	// so we progressively truncate until valid.
	lastValid := findLastValidPrefix(events)
	pruned := len(events) - lastValid

	// Truncate the in-memory events and persist.
	repaired := make([]eventlog.WireEvent, lastValid)
	copy(repaired, events[:lastValid])

	// Reload the full session metadata and messages, replace events, and save.
	// We can't use Load() because it would have rejected the corrupt spine.
	s, err := loadJSONLFile(path, sessionID)
	if err != nil {
		// Load failed because of the corrupt spine; we'll rebuild from raw.
		s = &Session{
			ID:       sessionID,
			Messages: nil,
			Events:   repaired,
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
		}
	} else {
		s.Events = repaired
	}
	s.UpdatedAt = time.Now()

	if err := Save(s); err != nil {
		return 0, fmt.Errorf("save repaired session: %w", err)
	}

	return pruned, nil
}

// forkEventsAt truncates a wire event spine to events that precede the message
// at atIndex. It maps message order to event sequence by counting UserMessage
// and AssistantMsg events (which carry model-visible Message payloads) and
// cutting after the atIndex-th such event. If the spine has no message-carrying
// events, all events are returned (the child gets a fresh seed boundary).
func forkEventsAt(events []eventlog.WireEvent, atIndex int) []eventlog.WireEvent {
	if len(events) == 0 {
		return nil
	}
	msgCount := 0
	cutSeq := uint64(0)
	for _, ev := range events {
		t := ev.Type
		if t == eventlog.UserMessage || t == eventlog.AssistantMsg {
			msgCount++
			if msgCount-1 <= atIndex { // 0-based: include events at indices 0..atIndex
				cutSeq = ev.Seq
			}
		}
	}
	if cutSeq == 0 {
		return nil // no correlating message event found
	}
	return truncateEventsAt(events, cutSeq)
}

// truncateEventsAt keeps events with Seq <= cutoff.
func truncateEventsAt(events []eventlog.WireEvent, cutoff uint64) []eventlog.WireEvent {
	if len(events) == 0 {
		return nil
	}
	for i, ev := range events {
		if ev.Seq > cutoff {
			if i == 0 {
				return nil
			}
			return events[:i]
		}
	}
	return events
}

// deriveMessageIndex counts model-visible message events (UserMessage,
// AssistantMsg) in the (already-truncated) event spine and returns the
// index of the last one. Returns -1 if no message events exist.
func deriveMessageIndex(events []eventlog.WireEvent) int {
	count := 0
	for _, ev := range events {
		t := ev.Type
		if t == eventlog.UserMessage || t == eventlog.AssistantMsg {
			count++
		}
	}
	if count == 0 {
		return -1
	}
	return count - 1
}

// findLastValidPrefix binary-searches for the longest prefix of events that
// passes eventlog.Validate (sequence monotonic + all types known). DecodeWire
// validates the entire slice; we progressively truncate until valid.
func findLastValidPrefix(events []eventlog.WireEvent) int {
	// Start from the full length and shrink until valid.
	for i := len(events); i > 0; i-- {
		if _, err := eventlog.DecodeWire(events[:i]); err == nil {
			return i
		}
	}
	return 0
}

func generateForkID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
