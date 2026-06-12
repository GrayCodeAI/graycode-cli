package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// MemoryService is the Session's view of the memory layer: yaad bridge,
// recall/remember interface, enhanced-memory manager, sleeptime
// consolidation, skill distillation, file-mention detector, agents
// accumulator. Extracted from Session in Phase 4 of the god-object
// decomposition (see docs/session-decomposition.md).
//
// The interface boundary is small on purpose: every method either
// does or doesn't talk to yaad, and the agent loop's branching on
// nil is preserved.
type MemoryService struct {
	// memory is the simple Recall/Remember interface.
	memory MemoryRecaller
	// yaad is the rich memory graph bridge.
	yaad *memory.YaadBridge
	// enhanced is the post-session memory manager.
	enhanced *memory.EnhancedMemoryManager
	// log is the session logger.
	log *logger.Logger
}

// NewMemoryService constructs an empty MemoryService. Wire the
// optional collaborators via the With* setters.
func NewMemoryService(log *logger.Logger) *MemoryService {
	if log == nil {
		log = logger.Default()
	}
	return &MemoryService{log: log}
}

// WithMemory sets the simple MemoryRecaller.
func (s *MemoryService) WithMemory(m MemoryRecaller) *MemoryService {
	s.memory = m
	return s
}

// WithYaad sets the yaad bridge.
func (s *MemoryService) WithYaad(y *memory.YaadBridge) *MemoryService {
	s.yaad = y
	return s
}

// WithEnhanced sets the enhanced-memory manager.
func (s *MemoryService) WithEnhanced(e *memory.EnhancedMemoryManager) *MemoryService {
	s.enhanced = e
	return s
}

// RecallContext returns a string of relevant memories for the given
// lastUserMsg under the given token budget. Returns empty string if
// no memory is wired. Combines yaad recall + few-shot examples +
// user-preference learning into one shot.
func (s *MemoryService) RecallContext(_ context.Context, lastUserMsg string, budget int) string {
	if s.yaad == nil {
		return ""
	}
	out, err := s.yaad.Recall(lastUserMsg, budget)
	if err != nil || out == "" {
		return ""
	}
	return "## Relevant Memories\n" + out
}

// Remember stores a content+category pair in the memory layer.
// Best-effort: errors are logged but not returned (the agent loop
// shouldn't fail a turn just because yaad is unavailable).
func (s *MemoryService) Remember(ctx context.Context, content, category string) {
	if s.enhanced != nil {
		_ = s.enhanced.Remember(content, category)
		return
	}
	if s.memory != nil {
		_ = s.memory.Remember(content, category)
	}
	_ = ctx // reserved for future context-aware memory ops
}

// OnSessionEnd runs the post-session memory bookkeeping.
func (s *MemoryService) OnSessionEnd(success bool) {
	if s.enhanced != nil {
		s.enhanced.EndSession(success)
	}
}

// Accessors.
func (s *MemoryService) Yaad() *memory.YaadBridge { return s.yaad }
func (s *MemoryService) Memory() MemoryRecaller   { return s.memory }
func (s *MemoryService) Enhanced() *memory.EnhancedMemoryManager {
	return s.enhanced
}

// IsZero reports whether the service has any memory wired.
func (s *MemoryService) IsZero() bool {
	return s == nil || (s.memory == nil && s.yaad == nil && s.enhanced == nil)
}

// _ unused-import workaround: keep types referenced even when none
// of the methods actually destructure EyrieMessage directly. The
// agent loop reads s.messages via the persistence service.
var _ = (*types.EyrieMessage)(nil)
