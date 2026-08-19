package engine

import (
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
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
	// stateMu protects compaction, token, and checkpoint metadata.
	stateMu sync.RWMutex
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
	// journal is the append-only session event spine. It is attached by the
	// composition root; nil means journaling is disabled (pure in-memory mode).
	journal *eventlog.Log
	// writeBehind buffers journal events and flushes them in batches via a
	// configured write function, porting DSH's SessionWriteBehind pattern.
	// Nil means write-behind batching is disabled (events are only durable
	// at explicit Save calls).
	writeBehind *session.WriteBehind
	// steering is the per-iteration user-guidance queue.
	steering *SteeringQueue
	// logger.
	log *logger.Logger
	// Compaction and checkpoint state belongs to persistence. Session keeps
	// deprecated aliases only for source compatibility with older callers.
	autoCompactor        *AutoCompactor
	files                *FileTracker
	persistID            string
	lastPromptTokens     int
	lastCompletionTokens int
	estTokensCache       int
	estTokensMsgCount    int
	estTokensLastLen     int
	checkpointMgr        *session.CheckpointManager
	onCompaction         OnCompaction
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

// Logger returns the logger used by persistence operations.
func (s *PersistenceService) Logger() *logger.Logger {
	if s == nil {
		return nil
	}
	return s.log
}

// SetLogger replaces the logger used by persistence operations.
func (s *PersistenceService) SetLogger(l *logger.Logger) {
	if s == nil {
		return
	}
	if l == nil {
		l = logger.Default()
	}
	s.log = l
}

// Messages returns a snapshot copy of the current transcript.
func (s *PersistenceService) Messages() []types.EyrieMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Read s.messages directly; the lock is already held. Calling
	// RawMessages() here would recursively RLock and can deadlock if a
	// writer arrives between the two read locks (Go's RWMutex forbids
	// recursive read-locking).
	return cloneMessages(s.messages)
}

// SetRawMessages replaces the message slice. Used by code paths
// that previously wrote to s.messages directly. The input is deep-copied so
// callers cannot mutate the live transcript after handing it to persistence.
// Safe on a nil receiver.
func (s *PersistenceService) SetRawMessages(msgs []types.EyrieMessage) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = cloneMessages(msgs)
	s.mu.Unlock()
}

// RawMessages returns an immutable snapshot of the transcript. Nested tool
// arguments and multimodal slices are cloned as well, so callers cannot
// mutate live session state by retaining or editing the returned value.
// Safe to call on a nil receiver (returns nil).
func (s *PersistenceService) RawMessages() []types.EyrieMessage {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMessages(s.messages)
}

// RawMessagesView returns the live transcript for read-only, non-retaining
// use. It performs no clone — repeated per-turn reads (token estimation,
// context management) must not deep-copy the whole transcript each time
// (M15 — that made long sessions quadratic). Callers must treat the result
// as ephemeral: appends may reallocate the backing array, and nested tool
// arguments are shared with live session state. Use RawMessages when a
// stable snapshot is required. Safe on a nil receiver (returns nil).
func (s *PersistenceService) RawMessagesView() []types.EyrieMessage {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messages
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
	s.AppendAssistantJournaled(types.EyrieMessage{Role: "assistant", Content: content})
}

// SetMessages replaces the transcript.
func (s *PersistenceService) SetMessages(msgs []types.EyrieMessage) {
	s.mu.Lock()
	s.messages = cloneMessages(msgs)
	s.mu.Unlock()
}

// ApplyCompaction atomically replaces the transcript with keep, preserving any
// messages appended concurrently since the compaction snapshot was taken.
// snapshotLen is the length of the snapshot keep was derived from; messages
// past that index are re-appended to the tail so a compaction racing an
// AddUser can never drop the new message. Safe on a nil receiver.
func (s *PersistenceService) ApplyCompaction(keep []types.EyrieMessage, snapshotLen int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if snapshotLen >= 0 && snapshotLen < len(s.messages) {
		kept := make([]types.EyrieMessage, 0, len(keep)+(len(s.messages)-snapshotLen))
		kept = append(kept, keep...)
		kept = append(kept, s.messages[snapshotLen:]...)
		s.messages = cloneMessages(kept)
		return
	}
	s.messages = cloneMessages(keep)
}

// AddUser appends a user message. Safe on a nil receiver.
func (s *PersistenceService) AddUser(content string) {
	s.AppendUserJournaled(types.EyrieMessage{Role: "user", Content: content})
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
	s.AppendUserJournaled(types.EyrieMessage{
		Role:    "user",
		Content: content,
		Images:  []string{dataURL},
	})
}

// AppendSystemContext appends a string to the system prompt.
func (s *PersistenceService) AppendSystemContext(content string) {
	if s == nil {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	s.mu.Lock()
	if strings.TrimSpace(s.system) == "" {
		s.system = content
	} else {
		s.system += "\n\n" + content
	}
	s.mu.Unlock()
	// Emit context.injected so the model-visible surface is reconstructible
	// from the log alone (DSH context.injected seam).
	if j := s.Journal(); j != nil {
		j.AppendContextInjected(content)
	}
}

// ReplaceSystemContextSection replaces a section of the system prompt
// identified by a header string. Used by yaad recall (which refreshes
// the "## Relevant Memories" block on every turn).
func (s *PersistenceService) ReplaceSystemContextSection(header, content string) {
	if s == nil {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := strings.Index(s.system, header)
	if idx < 0 {
		if strings.TrimSpace(s.system) == "" {
			s.system = content
		} else {
			s.system += "\n\n" + content
		}
		return
	}
	rest := s.system[idx+len(header):]
	var end int
	if next := strings.Index(rest, "\n\n## "); next >= 0 {
		end = idx + len(header) + next
	} else {
		// Replace the entire existing section, retaining the next section
		// separator when one exists.
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
	// Clone while holding the lock; callers retain no mutable alias to the
	// live transcript.
	s.messages = cloneMessages(msgs)
	s.mu.Unlock()
}

// cloneMessages performs a deep copy of the provider-neutral transcript.
// Tool arguments are arbitrary JSON-shaped values, so cloneJSONValue walks
// maps and slices recursively instead of relying on a shallow slice copy.
func cloneMessages(in []types.EyrieMessage) []types.EyrieMessage {
	if in == nil {
		return nil
	}
	out := make([]types.EyrieMessage, len(in))
	for i, msg := range in {
		out[i] = msg
		out[i].Images = append([]string(nil), msg.Images...)
		out[i].ContentParts = append([]types.ContentPart(nil), msg.ContentParts...)
		if msg.ToolUse != nil {
			out[i].ToolUse = make([]types.ToolCall, len(msg.ToolUse))
			for j, call := range msg.ToolUse {
				out[i].ToolUse[j] = call
				if call.Arguments != nil {
					out[i].ToolUse[j].Arguments = make(map[string]interface{}, len(call.Arguments))
					for key, value := range call.Arguments {
						out[i].ToolUse[j].Arguments[key] = cloneJSONValue(value)
					}
				}
			}
		}
		out[i].ToolResults = append([]types.ToolResult(nil), msg.ToolResults...)
	}
	return out
}

func cloneJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, nested := range v {
			out[key] = cloneJSONValue(nested)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, nested := range v {
			out[i] = cloneJSONValue(nested)
		}
		return out
	default:
		return value
	}
}

// PinnedMessages returns the count of pinned messages.
func (s *PersistenceService) PinnedMessages() int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.pinnedMessages
}

// SetPinnedMessages replaces the pinned count.
func (s *PersistenceService) SetPinnedMessages(n int) {
	s.stateMu.Lock()
	s.pinnedMessages = n
	s.stateMu.Unlock()
}

// AutoCompactThresholdPct returns the auto-compact threshold %.
func (s *PersistenceService) AutoCompactThresholdPct() int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.autoCompactThresholdPct
}

// SetAutoCompactThresholdPct replaces the auto-compact threshold %.
func (s *PersistenceService) SetAutoCompactThresholdPct(pct int) {
	s.stateMu.Lock()
	s.autoCompactThresholdPct = pct
	s.stateMu.Unlock()
}

// ContextWindowCached returns the catalog context window.
func (s *PersistenceService) ContextWindowCached() int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.contextWindowCached
}

// SetContextWindowCached replaces the catalog context window.
func (s *PersistenceService) SetContextWindowCached(n int) {
	s.stateMu.Lock()
	s.contextWindowCached = n
	s.stateMu.Unlock()
}

func (s *PersistenceService) AutoCompactor() *AutoCompactor {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.autoCompactor
}

func (s *PersistenceService) SetAutoCompactor(ac *AutoCompactor) {
	s.stateMu.Lock()
	s.autoCompactor = ac
	s.stateMu.Unlock()
}

func (s *PersistenceService) Files() *FileTracker {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.files
}

func (s *PersistenceService) SetFiles(files *FileTracker) {
	s.stateMu.Lock()
	s.files = files
	s.stateMu.Unlock()
}

func (s *PersistenceService) PersistID() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.persistID
}

func (s *PersistenceService) SetPersistID(id string) {
	s.stateMu.Lock()
	s.persistID = id
	s.stateMu.Unlock()
}

func (s *PersistenceService) LastPromptTokens() int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.lastPromptTokens
}

func (s *PersistenceService) LastCompletionTokens() int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.lastCompletionTokens
}

func (s *PersistenceService) SetTokenUsage(prompt, completion int) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if prompt > 0 {
		s.lastPromptTokens = prompt
	}
	if completion > 0 {
		s.lastCompletionTokens = completion
	}
}

func (s *PersistenceService) TokenEstimateCache() (tokens, count, lastLen int) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.estTokensCache, s.estTokensMsgCount, s.estTokensLastLen
}

func (s *PersistenceService) SetTokenEstimateCache(tokens, count, lastLen int) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.estTokensCache, s.estTokensMsgCount, s.estTokensLastLen = tokens, count, lastLen
}

func (s *PersistenceService) CheckpointManager() *session.CheckpointManager {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.checkpointMgr
}

func (s *PersistenceService) SetCheckpointManager(cm *session.CheckpointManager) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.checkpointMgr = cm
}

func (s *PersistenceService) OnCompaction() OnCompaction {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.onCompaction
}

func (s *PersistenceService) SetOnCompaction(fn OnCompaction) {
	s.stateMu.Lock()
	s.onCompaction = fn
	s.stateMu.Unlock()
}
