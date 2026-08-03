package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/engine/ctxmgr"
	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
)

const (
	// DefaultContextWindow is used when the model catalog has no context size.
	DefaultContextWindow = 128_000
	// DefaultAutoCompactThresholdPct matches Grok CLI default (85% of window).
	DefaultAutoCompactThresholdPct = 85
)

// ResolveModelContextWindow returns the effective context window for a model.
func ResolveModelContextWindow(model string, override int) int {
	if override > 0 {
		return override
	}
	if info, ok := modelPkg.Find(model); ok && info.ContextSize > 0 {
		return info.ContextSize
	}
	return DefaultContextWindow
}

// ContextWindowSize returns this session's context window (catalog or default).
func (s *Session) ContextWindowSize() int {
	if s == nil {
		return DefaultContextWindow
	}
	if w := s.ContextWindowCachedValue(); w > 0 {
		return w
	}
	model := ""
	if s.ChatLLM() != nil {
		model = s.ChatLLM().Model()
	}
	return ResolveModelContextWindow(model, 0)
}

// EnsureAutoCompactor initializes the compaction orchestrator from session settings.
func (s *Session) EnsureAutoCompactor() {
	if s == nil {
		return
	}
	p := s.Persistence()
	if p == nil {
		return
	}
	if p.AutoCompactor() != nil {
		p.AutoCompactor().Configure(s.compactConfig())
		return
	}
	p.SetAutoCompactor(NewAutoCompactor(s.compactConfig()))
}

func (s *Session) compactThresholdPct() int {
	pct := s.Persistence().AutoCompactThresholdPct()
	if pct <= 0 {
		pct = DefaultAutoCompactThresholdPct
	}
	if pct < 50 {
		pct = 50
	}
	if pct > 95 {
		pct = 95
	}
	return pct
}

func (s *Session) compactConfig() CompactConfig {
	window := s.ContextWindowSize()
	pct := s.compactThresholdPct()
	target := window * pct / 100
	cfg := DefaultCompactConfig()
	cfg.AutoEnabled = true
	cfg.ContextWindowSize = window
	cfg.MaxOutputTokens = 0
	cfg.AutoCompactBuffer = window - target
	if cfg.AutoCompactBuffer < 0 {
		cfg.AutoCompactBuffer = 0
	}
	return cfg
}

// refreshContextWindowCache updates cached window from the catalog when the model changes.
func (s *Session) refreshContextWindowCache() {
	if s == nil {
		return
	}
	if s.Persistence() == nil {
	} else {
		s.SetContextWindowCached(0)
	}
	model := ""
	if s.ChatLLM() != nil {
		model = s.ChatLLM().Model()
	}
	if info, ok := modelPkg.Find(model); ok && info.ContextSize > 0 {
		if s.Persistence() == nil {
		} else {
			s.SetContextWindowCached(info.ContextSize)
		}
	}
	s.EnsureAutoCompactor()
}

// WillCompactBeforeTurn reports whether the next ManageContextBeforeTurn call will compact.
func (s *Session) WillCompactBeforeTurn() bool {
	if s == nil {
		return false
	}
	s.EnsureAutoCompactor()
	if s.Persistence().AutoCompactor().ShouldAutoCompact(s) {
		return true
	}
	// Read-only view: repeated per-turn reads must not deep-clone the whole
	// transcript (M15 — RawMessages was O(n) per read, O(n²) per session).
	if len(s.Persistence().RawMessagesView()) > maxContextMessages {
		return true
	}
	convTokens := EstimateTokens(s.Persistence().RawMessagesView())
	budget := ctxmgr.NewContextBudget(s.ContextWindowSize())
	return budget.ShouldCompact(convTokens)
}

// ManageContextBeforeTurn collapses noise, then compacts via the strategy registry when needed.
// Returns the compaction strategy name (if any) and whether messages were reduced.
func (s *Session) ManageContextBeforeTurn(ctx context.Context) (strategy string, compacted bool) {
	if s == nil {
		return "", false
	}
	s.Persistence().SetRawMessages(ctxmgr.CollapseRepeatedMessages(s.Persistence().RawMessages()))

	s.EnsureAutoCompactor()
	if compactStrategy, ok := s.Persistence().AutoCompactor().AutoCompactIfNeeded(ctx, s); ok {
		return compactStrategy, true // recordCompaction emitted inside AutoCompactIfNeeded
	}

	if len(s.Persistence().RawMessagesView()) > maxContextMessages {
		before := EstimateTokens(s.Persistence().RawMessagesView())
		s.smartCompact()
		s.recordCompaction("smart_message_cap", before, EstimateTokens(s.Persistence().RawMessagesView()), false)
		return "smart_message_cap", true
	}

	convTokens := EstimateTokens(s.Persistence().RawMessagesView())
	window := s.ContextWindowSize()
	budget := ctxmgr.NewContextBudget(window)
	if budget.ShouldCompact(convTokens) {
		before := EstimateTokens(s.Persistence().RawMessagesView())
		s.smartCompact()
		s.recordCompaction("smart_budget", before, EstimateTokens(s.Persistence().RawMessagesView()), false)
		return "smart_budget", true
	}

	return "", false
}

// CompactConversation runs compaction immediately (for /compact). Uses the full strategy chain.
func (s *Session) CompactConversation(ctx context.Context) (strategy string, tokensBefore, tokensAfter int, err error) {
	if s == nil {
		return "", 0, 0, fmt.Errorf("no session")
	}
	s.Persistence().SetRawMessages(ctxmgr.CollapseRepeatedMessages(s.Persistence().RawMessages()))
	s.EnsureAutoCompactor()
	tokensBefore = EstimateTokens(s.Persistence().RawMessages())
	strategy, err = s.Persistence().AutoCompactor().RunCompaction(ctx, s)
	if err != nil {
		s.smartCompact()
		strategy = "smart_fallback"
	}
	tokensAfter = EstimateTokens(s.Persistence().RawMessages())
	s.recordCompaction(strategy, tokensBefore, tokensAfter, true)
	return strategy, tokensBefore, tokensAfter, nil
}

// ShouldCompactByBudget reports whether conversation tokens exceed the configured % of window.
func (s *Session) ShouldCompactByBudget() bool {
	window := s.ContextWindowSize()
	conv := EstimateTokens(s.Persistence().RawMessages())
	return conv >= window*s.compactThresholdPct()/100
}
