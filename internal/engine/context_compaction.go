package engine

import (
	"path/filepath"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// CompactionEvent describes a completed context compaction.
type CompactionEvent struct {
	Strategy     string
	TokensBefore int
	TokensAfter  int
	Manual       bool
}

// OnCompaction is invoked after compaction (TUI, logging, etc.).
type OnCompaction func(CompactionEvent)

// PersistID is the on-disk session id under Hawk's user state sessions dir.
// Set from the TUI when a chat session is created or resumed.
func (s *Session) SetPersistID(id string) {
	if s == nil {
		return
	}
	if p := s.Persistence(); p != nil {
		p.SetPersistID(id)
		p.SetCheckpointManager(nil)
	}
	s.ConfigureContextGraphObservation("")
}

// RecordAPIUsage stores the latest provider prompt/completion token counts.
func (s *Session) RecordAPIUsage(prompt, completion int) {
	if s == nil {
		return
	}
	if p := s.Persistence(); p != nil {
		p.SetTokenUsage(prompt, completion)
	}
}

// LastPromptTokens returns the most recent API prompt token count (0 if unknown).
func (s *Session) LastPromptTokens() int {
	if s == nil {
		return 0
	}
	if p := s.Persistence(); p != nil {
		return p.LastPromptTokens()
	}
	return 0
}

// ContextUsedTokens returns API prompt tokens when available, else an estimate.
func (s *Session) ContextUsedTokens() int {
	if p := s.LastPromptTokens(); p > 0 {
		return p
	}
	persist := s.Persistence()
	if persist == nil {
		return 0
	}
	msgs := persist.RawMessages()
	count := len(msgs)
	var lastLen int
	if count > 0 {
		lastLen = len(msgs[count-1].Content)
	}

	if cache, cachedCount, cachedLen := persist.TokenEstimateCache(); cachedCount == count && cachedLen == lastLen && cache > 0 {
		return cache
	}

	est := EstimateTokens(msgs)
	persist.SetTokenEstimateCache(est, count, lastLen)

	return est
}

func (s *Session) notifyCompaction(ev CompactionEvent) {
	if s == nil {
		return
	}
	if fn := s.Persistence().OnCompaction(); fn != nil {
		fn(ev)
	}
	s.saveCompactionCheckpoint()
}

func (s *Session) saveCompactionCheckpoint() {
	dir := s.checkpointDir()
	if dir == "" {
		return
	}
	cm := s.checkpointManager()
	if cm == nil {
		return
	}
	msgs := rawToSessionMessages(s.RawMessages())
	_, _ = cm.AutoCheckpoint(msgs, nil)
}

func (s *Session) checkpointDir() string {
	if s == nil {
		return ""
	}
	id := s.Persistence().PersistID()
	if id == "" {
		return ""
	}
	return filepath.Join(storage.SessionsDir(), id, "checkpoints")
}

func (s *Session) checkpointManager() *session.CheckpointManager {
	if s == nil {
		return nil
	}
	dir := s.checkpointDir()
	if dir == "" {
		return nil
	}
	p := s.Persistence()
	if p == nil {
		return nil
	}
	if p.CheckpointManager() == nil {
		cm := session.NewCheckpointManager(dir)
		_ = cm.Load()
		p.SetCheckpointManager(cm)
	}
	return p.CheckpointManager()
}

func rawToSessionMessages(raw []types.EyrieMessage) []session.Message {
	return session.FromRuntimeMessages(raw)
}

func (s *Session) recordCompaction(strategy string, tokensBefore, tokensAfter int, manual bool) {
	if s == nil || strategy == "" {
		return
	}
	s.notifyCompaction(CompactionEvent{
		Strategy:     strategy,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Manual:       manual,
	})
}
