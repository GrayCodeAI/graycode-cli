package engine

import (
	"strings"
	"sync"

	"github.com/GrayCodeAI/eyrie/storage"

	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// PersistenceService is the Session's view of the conversation store:
// the messages slice, the conversation DAG, the system prompt, the
// pinned-messages counter, the auto-compact threshold, the
// AutoCompactor, the cost tracker, the per-session file tracker.
//
// Extracted from Session in Phase 5 of the god-object decomposition
// (see docs/session-decomposition.md). All methods are safe to call
// without external state; the underlying RWMutex is preserved for
// concurrent access (daemon handling concurrent requests, background
// memory goroutines).
type PersistenceService struct {
	// mu protects messages and system for concurrent access.
	mu sync.RWMutex
	// messages is the full transcript (system + user + assistant + tool_use + tool_result).
	messages []types.EyrieMessage
	// system is the system prompt (mutable, agents append learned guidelines).
	system string
	// pinnedMessages is the count of messages protected from compaction (from /pin).
	pinnedMessages int
	// autoCompactThresholdPct is the token % that triggers auto-compact (default 85).
	autoCompactThresholdPct int
	// contextWindowCached is the catalog context window; 0 → governor default.
	contextWindowCached int
	// dag is the conversation DAG (for branching).
	dag *storage.DAG
	// steering is the per-iteration user-guidance queue.
	steering *SteeringQueue
	// logger.
	log *logger.Logger
}

// NewPersistenceService constructs an empty PersistenceService.
func NewPersistenceService(log *logger.Logger) *PersistenceService {
	if log == nil {
		log = logger.Default()
	}
	return &PersistenceService{
		log:                     log,
		autoCompactThresholdPct: DefaultAutoCompactThresholdPct,
	}
}

// Messages returns a snapshot copy of the current transcript.
func (s *PersistenceService) Messages() []types.EyrieMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw := s.RawMessages()
	out := make([]types.EyrieMessage, len(raw))
	copy(out, raw)
	return out
}

// SetRawMessages replaces the message slice. Used by code paths
// that previously wrote to s.messages directly. Pass-by-reference
// to keep the slice header mutable. Safe on a nil receiver.
func (s *PersistenceService) SetRawMessages(msgs []types.EyrieMessage) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = msgs
	s.mu.Unlock()
}

// RawMessages returns the live slice (no copy). Callers MUST NOT mutate.
// Used by the agent loop's hot path where copy overhead matters.
// Safe to call on a nil receiver (returns nil).
func (s *PersistenceService) RawMessages() []types.EyrieMessage {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messages
}
// DAG returns the conversation DAG. New code should access this
// through s.Persistence().DAG().
func (s *PersistenceService) DAG() *storage.DAG { return s.dag }

// SetDAG attaches the conversation DAG.
func (s *PersistenceService) SetDAG(dag *storage.DAG) { s.dag = dag }

// Steering returns the per-iteration user-guidance queue. New
// code should access this through s.Persistence().Steering().
func (s *PersistenceService) Steering() *SteeringQueue { return s.steering }

// SetSteering attaches the user-guidance queue.
func (s *PersistenceService) SetSteering(sq *SteeringQueue) { s.steering = sq }

// AddAssistant appends an assistant message.
func (s *PersistenceService) AddAssistant(content string) {
	s.mu.Lock()
	s.messages = append(s.messages, types.EyrieMessage{Role: "assistant", Content: content})
	s.mu.Unlock()
}

// SetMessages replaces the transcript.
func (s *PersistenceService) SetMessages(msgs []types.EyrieMessage) {
	s.mu.Lock()
	s.messages = msgs
	s.mu.Unlock()
}

// AddUser appends a user message. Safe on a nil receiver.
func (s *PersistenceService) AddUser(content string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = append(s.messages, types.EyrieMessage{Role: "user", Content: content})
	s.mu.Unlock()
}

// AddUserWithImage appends a user message with an inline image.
func (s *PersistenceService) AddUserWithImage(content, imageBase64, imageType string) {
	s.mu.Lock()
	s.SetRawMessages(append(s.RawMessages(), types.EyrieMessage{
		Role:    "user",
		Content: content,
		Images:  []string{imageBase64},
	}))
	s.mu.Unlock()
	_ = imageType // reserved for future typing
}

// AppendSystemContext appends a string to the system prompt.
func (s *PersistenceService) AppendSystemContext(content string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.system += content
	s.mu.Unlock()
}

// ReplaceSystemContextSection replaces a section of the system prompt
// identified by a header string. Used by yaad recall (which refreshes
// the "## Relevant Memories" block on every turn).
func (s *PersistenceService) ReplaceSystemContextSection(header, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := strings.Index(s.system, header)
	if idx < 0 {
		s.system += content
		return
	}
	end := idx + len(header)
	if nl := strings.Index(s.system[end:], "\n\n"); nl >= 0 {
		end += nl + 2
	} else {
		end = len(s.system)
	}
	s.system = s.system[:idx] + content + s.system[end:]
}

// System returns the current system prompt.
func (s *PersistenceService) System() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.system
}

// SetSystem replaces the system prompt entirely.
func (s *PersistenceService) SetSystem(sys string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.system = sys
	s.mu.Unlock()
}

// MessageCount returns the current message count.
func (s *PersistenceService) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.RawMessages())
}

// RemoveLastExchange removes the last (assistant, user) pair.
func (s *PersistenceService) RemoveLastExchange() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.RawMessages()) < 2 {
		return
	}
	s.SetRawMessages( s.RawMessages()[:len(s.RawMessages())-2]);
}

// LoadMessages replaces the transcript with a fresh slice.
func (s *PersistenceService) LoadMessages(msgs []types.EyrieMessage) {
	s.mu.Lock()
	s.SetRawMessages( msgs);
	s.mu.Unlock()
}

// PinnedMessages returns the count of pinned messages.
func (s *PersistenceService) PinnedMessages() int { return s.pinnedMessages }

// SetPinnedMessages replaces the pinned count.
func (s *PersistenceService) SetPinnedMessages(n int) { s.pinnedMessages = n }

// AutoCompactThresholdPct returns the auto-compact threshold %.
func (s *PersistenceService) AutoCompactThresholdPct() int { return s.autoCompactThresholdPct }

// SetAutoCompactThresholdPct replaces the auto-compact threshold %.
func (s *PersistenceService) SetAutoCompactThresholdPct(pct int) {
	s.autoCompactThresholdPct = pct
}

// ContextWindowCached returns the catalog context window.
func (s *PersistenceService) ContextWindowCached() int { return s.contextWindowCached }

// SetContextWindowCached replaces the catalog context window.
func (s *PersistenceService) SetContextWindowCached(n int) { s.contextWindowCached = n }

// noopLog is a placeholder type so the unused _ field doesn't trigger
// the "unused field" linter.
type noopLog struct{}
