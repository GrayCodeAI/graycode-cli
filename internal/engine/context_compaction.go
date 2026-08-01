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
	s.mu.Lock()
	s.persistID = id
	s.checkpointMgr = nil
	s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if prompt > 0 {
		s.lastPromptTokens = prompt
	}
	if completion > 0 {
		s.lastCompletionTokens = completion
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastPromptTokens
}

// ContextUsedTokens returns API prompt tokens when available, else an estimate.
func (s *Session) ContextUsedTokens() int {
	if p := s.LastPromptTokens(); p > 0 {
		return p
	}
	msgs := s.Persistence().RawMessages()
	count := len(msgs)
	var lastLen int
	if count > 0 {
		lastLen = len(msgs[count-1].Content)
	}

	if p := s.Persistence(); p != nil {
		if cache, cachedCount, cachedLen := p.TokenEstimateCache(); cachedCount == count && cachedLen == lastLen && cache > 0 {
			return cache
		}
	}

	est := EstimateTokens(msgs)

	if p := s.Persistence(); p != nil {
		p.SetTokenEstimateCache(est, count, lastLen)
	} else {
		s.mu.Lock()
		s.estTokensMsgCount = count
		s.estTokensLastLen = lastLen
		s.estTokensCache = est
		s.mu.Unlock()
	}

	return est
}

func (s *Session) notifyCompaction(ev CompactionEvent) {
	if s == nil {
		return
	}
	if fn := s.Persistence().OnCompaction(); fn != nil {
		fn(ev)
	} else if s.OnCompaction != nil {
		s.OnCompaction(ev)
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
