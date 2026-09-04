package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/memory"
	"github.com/GrayCodeAI/graycode-cli/internal/observability/logger"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// MemoryService is the Session's view of the memory layer: harrier bridge,
// recall/remember interface, enhanced-memory manager, sleeptime
// consolidation, skill distillation, file-mention detector, agents
// accumulator. Extracted from Session in Phase 4 of the god-object
// decomposition (see docs/session-decomposition.md).
//
// The interface boundary is small on purpose: every method either
// does or doesn't talk to harrier, and the agent loop's branching on
// nil is preserved.
type MemoryService struct {
	// memory is the simple Recall/Remember interface.
	memory MemoryRecaller
	// harrier is the rich memory graph bridge.
	harrier *memory.HarrierBridge
	// enhanced is the post-session memory manager.
	enhanced *memory.EnhancedMemoryManager
	// skillDistiller produces reusable skill patterns from past
	// session tool-call sequences.
	skillDistiller *memory.SkillDistiller
	// sleeptime runs background memory consolidation.
	sleeptime *memory.SleeptimeAgent
	// activity tracks memory-save nudges (Engram pattern).
	activity *memory.ActivityTracker
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

// Logger returns the logger shared by memory collaborators.
func (s *MemoryService) Logger() *logger.Logger {
	if s == nil {
		return nil
	}
	return s.log
}

// SetLogger replaces the logger shared by memory collaborators.
func (s *MemoryService) SetLogger(l *logger.Logger) {
	if s == nil {
		return
	}
	if l == nil {
		l = logger.Default()
	}
	s.log = l
}

// WithMemory sets the simple MemoryRecaller.
func (s *MemoryService) WithMemory(m MemoryRecaller) *MemoryService {
	s.memory = m
	return s
}

// WithHarrier sets the harrier bridge.
func (s *MemoryService) WithHarrier(y *memory.HarrierBridge) *MemoryService {
	s.harrier = y
	return s
}

// WithEnhanced sets the enhanced-memory manager.
func (s *MemoryService) WithEnhanced(e *memory.EnhancedMemoryManager) *MemoryService {
	s.enhanced = e
	return s
}

// RecallContext returns a string of relevant memories for the given
// lastUserMsg under the given token budget. Returns empty string if
// no memory is wired. Combines harrier recall + few-shot examples +
// user-preference learning into one shot.
func (s *MemoryService) RecallContext(_ context.Context, lastUserMsg string, budget int) string {
	if s == nil {
		return ""
	}
	var out string
	if s.harrier != nil {
		out, _ = s.harrier.Recall(lastUserMsg, budget)
	}
	// The simple recaller is the compatibility path used by tests and
	// lightweight integrations that do not install Harrier. Memory ownership
	// stays in this service instead of leaking a backend decision into the
	// agent loop.
	if out == "" && s.memory != nil {
		out, _ = s.memory.Recall(lastUserMsg, budget)
	}
	if out == "" {
		return ""
	}
	return "## Relevant Memories\n" + out
}

// Remember stores a content+category pair in the memory layer.
// Best-effort: errors are logged but not returned (the agent loop
// shouldn't fail a turn just because harrier is unavailable).
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

// Finalize performs memory-side session bookkeeping from a transcript
// snapshot. The agent loop does not need to know which backend is installed.
func (s *MemoryService) Finalize(messages []types.GraycodeRouterMessage, success bool) {
	if s == nil {
		return
	}
	if s.enhanced != nil {
		s.enhanced.EndSession(success)
	}
	if s.memory == nil {
		return
	}
	goal := ""
	for _, message := range messages {
		if message.Role == "user" && len(message.ToolResults) == 0 {
			goal = message.Content
			break
		}
	}
	if goal != "" {
		summary := fmt.Sprintf("Session goal: %s", goal)
		if !success {
			summary += " (interrupted)"
		}
		_ = s.memory.Remember(summary, "session")
	}
}

// Accessors.
func (s *MemoryService) Harrier() *memory.HarrierBridge { return s.harrier }
func (s *MemoryService) Memory() MemoryRecaller         { return s.memory }
func (s *MemoryService) Enhanced() *memory.EnhancedMemoryManager {
	return s.enhanced
}

// SetMemory replaces the legacy memory implementation. Used by
// external packages that previously wrote to sess.Memory directly.
// Both views stay in sync: the Session.Memory alias points to the
// same value.
func (s *MemoryService) SetMemory(m MemoryRecaller) { s.memory = m }

// SetHarrier replaces the legacy Harrier bridge.
func (s *MemoryService) SetHarrier(y *memory.HarrierBridge) { s.harrier = y }

// SetEnhanced replaces the legacy enhanced memory manager.
func (s *MemoryService) SetEnhanced(e *memory.EnhancedMemoryManager) { s.enhanced = e }

// SetSkillDistiller replaces the legacy skill distiller.
func (s *MemoryService) SetSkillDistiller(sd *memory.SkillDistiller) { s.skillDistiller = sd }

// SetSleeptime replaces the legacy background consolidator.
func (s *MemoryService) SetSleeptime(st *memory.SleeptimeAgent) { s.sleeptime = st }

// SetActivity replaces the legacy activity tracker.
func (s *MemoryService) SetActivity(act *memory.ActivityTracker) { s.activity = act }

// SkillDistiller returns the legacy skill distiller. New code
// should access this through s.MemorySvc().SkillDistiller().
func (s *MemoryService) SkillDistiller() *memory.SkillDistiller { return s.skillDistiller }

// Sleeptime returns the background memory consolidator. New code
// should access this through s.MemorySvc().Sleeptime().
func (s *MemoryService) Sleeptime() *memory.SleeptimeAgent { return s.sleeptime }

// Activity returns the memory-save activity nudger. New code
// should access this through s.MemorySvc().Activity().
func (s *MemoryService) Activity() *memory.ActivityTracker { return s.activity }

// IsZero reports whether the service has any memory wired.
func (s *MemoryService) IsZero() bool {
	return s == nil || (s.memory == nil && s.harrier == nil && s.enhanced == nil && s.skillDistiller == nil && s.sleeptime == nil && s.activity == nil)
}
