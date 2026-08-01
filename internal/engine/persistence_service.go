package engine

import (
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/session"
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
	// graph is Hawk's product-owned conversation graph (for branching).
	graph *session.ConversationGraph
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
	return cloneMessages(s.messages)
}

// SetRawMessages replaces the message slice. Used by code paths
// that previously wrote to s.messages directly. The input is copied so
// callers cannot mutate the persisted transcript after publication.
func (s *PersistenceService) SetRawMessages(msgs []types.EyrieMessage) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = cloneMessages(msgs)
	s.mu.Unlock()
}

// RawMessages returns a snapshot copy of the transcript. Returning the live
// slice was race-prone because background work could retain it after the lock
// was released. Safe to call on a nil receiver (returns nil).
func (s *PersistenceService) RawMessages() []types.EyrieMessage {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMessages(s.messages)
}

// Graph returns Hawk's product-owned conversation graph.
func (s *PersistenceService) Graph() *session.ConversationGraph { return s.graph }

// SetGraph attaches the conversation graph.
func (s *PersistenceService) SetGraph(graph *session.ConversationGraph) { s.graph = graph }

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
	s.messages = cloneMessages(msgs)
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
// The image is stored as a data URL ("data:<imageType>;base64,<base64>")
// so the LLM-side eyrie client can decode it from the message body
// without any extra metadata channel. imageType is "image/png",
// "image/jpeg", etc.
func (s *PersistenceService) AddUserWithImage(content, imageBase64, imageType string) {
	if s == nil {
		return
	}
	dataURL := "data:" + imageType + ";base64," + imageBase64
	s.mu.Lock()
	s.messages = append(s.messages, types.EyrieMessage{
		Role:    "user",
		Content: content,
		Images:  []string{dataURL},
	})
	s.mu.Unlock()
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
	// Direct field read; lock already held (avoid recursive RLock).
	return len(s.messages)
}

// RemoveLastExchange removes the last (assistant, user) pair.
func (s *PersistenceService) RemoveLastExchange() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Operate on s.messages directly: the write lock is held, so calling
	// the RawMessages()/SetRawMessages() accessors (which lock again) would
	// deadlock.
	if len(s.messages) < 2 {
		return
	}
	s.messages = s.messages[:len(s.messages)-2]
}

// LoadMessages replaces the transcript with a fresh slice.
func (s *PersistenceService) LoadMessages(msgs []types.EyrieMessage) {
	s.mu.Lock()
	s.messages = cloneMessages(msgs)
	s.mu.Unlock()
}

// cloneMessages copies the message slice and its mutable nested slices/maps.
// ContentParts are interface values and are copied at the slice boundary; the
// concrete content blocks remain owned by their callers.
func cloneMessages(msgs []types.EyrieMessage) []types.EyrieMessage {
	if msgs == nil {
		return nil
	}
	out := make([]types.EyrieMessage, len(msgs))
	for i, msg := range msgs {
		out[i] = msg
		out[i].Images = append([]string(nil), msg.Images...)
		out[i].ContentParts = append([]types.ContentPart(nil), msg.ContentParts...)
		out[i].ToolUse = cloneToolCalls(msg.ToolUse)
		out[i].ToolResults = append([]types.ToolResult(nil), msg.ToolResults...)
	}
	return out
}

func cloneToolCalls(calls []types.ToolCall) []types.ToolCall {
	if calls == nil {
		return nil
	}
	out := make([]types.ToolCall, len(calls))
	for i, call := range calls {
		out[i] = call
		if call.Arguments != nil {
			out[i].Arguments = make(map[string]interface{}, len(call.Arguments))
			for key, value := range call.Arguments {
				out[i].Arguments[key] = value
			}
		}
	}
	return out
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
