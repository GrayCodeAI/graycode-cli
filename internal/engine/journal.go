package engine

import (
	"github.com/GrayCodeAI/hawk/internal/eventlog"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// SetJournal attaches the append-only session event spine to persistence. New
// model-visible appends should go through AppendUserJournaled /
// AppendAssistantJournaled so the journal and the live transcript stay in sync.
func (s *PersistenceService) SetJournal(j *eventlog.Log) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	s.journal = j
	s.stateMu.Unlock()
}

// Journal returns the attached event spine, or nil.
func (s *PersistenceService) Journal() *eventlog.Log {
	if s == nil {
		return nil
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.journal
}

// AppendUserJournaled journals a user message and appends it to the transcript.
// Safe on a nil receiver.
func (s *PersistenceService) AppendUserJournaled(msg types.EyrieMessage) {
	if s == nil {
		return
	}
	if j := s.Journal(); j != nil {
		j.Append(eventlog.UserMessage, toJournalMessage(msg))
	}
	s.mu.Lock()
	s.messages = append(s.messages, cloneSingleMessage(msg))
	s.mu.Unlock()
}

// AppendAssistantJournaled journals an assistant message and appends it to the
// transcript. Safe on a nil receiver.
func (s *PersistenceService) AppendAssistantJournaled(msg types.EyrieMessage) {
	if s == nil {
		return
	}
	if j := s.Journal(); j != nil {
		j.Append(eventlog.AssistantMsg, toJournalMessage(msg))
	}
	s.mu.Lock()
	s.messages = append(s.messages, cloneSingleMessage(msg))
	s.mu.Unlock()
}

// JournalProjection returns the model-visible message surface rebuilt from the
// event spine. It is the reference projection the invariant checks against
// Messages(); compactions and other non-append transcript edits are not yet
// represented as journal events, so consistency is only expected for transcripts
// built exclusively through the journaled append methods.
func (s *PersistenceService) JournalProjection() []types.EyrieMessage {
	if s == nil {
		return nil
	}
	j := s.Journal()
	if j == nil {
		return nil
	}
	msgs := eventlog.ProjectMessages(j.Snapshot())
	out := make([]types.EyrieMessage, len(msgs))
	for i, m := range msgs {
		out[i] = fromJournalMessage(m)
	}
	return out
}

func toJournalMessage(m types.EyrieMessage) eventlog.Message {
	jm := eventlog.Message{
		Role:     m.Role,
		Content:  m.Content,
		Thinking: m.Thinking,
		Images:   append([]string(nil), m.Images...),
	}
	for _, part := range m.ContentParts {
		p := eventlog.ContentPartPayload{Type: part.Type, Text: part.Text}
		if part.ImageURL != nil {
			p.ImageURL = part.ImageURL.URL
			p.ImageDetail = part.ImageURL.Detail
		}
		if part.InputAudio != nil {
			p.AudioData = part.InputAudio.Data
			p.AudioFormat = part.InputAudio.Format
		}
		jm.ContentParts = append(jm.ContentParts, p)
	}
	for _, call := range m.ToolUse {
		jm.ToolUse = append(jm.ToolUse, eventlog.ToolCallPayload{
			ID: call.ID, Name: call.Name, Arguments: call.Arguments,
		})
	}
	for _, res := range m.ToolResults {
		jm.ToolResults = append(jm.ToolResults, eventlog.ToolResultPayload{
			ToolUseID: res.ToolUseID, Content: res.Content, IsError: res.IsError,
		})
	}
	return jm
}

func fromJournalMessage(m eventlog.Message) types.EyrieMessage {
	out := types.EyrieMessage{
		Role:     m.Role,
		Content:  m.Content,
		Thinking: m.Thinking,
		Images:   append([]string(nil), m.Images...),
	}
	for _, p := range m.ContentParts {
		part := types.ContentPart{Type: p.Type, Text: p.Text}
		if p.ImageURL != "" {
			part.ImageURL = &types.ImageURLPart{URL: p.ImageURL, Detail: p.ImageDetail}
		}
		if p.AudioData != "" || p.AudioFormat != "" {
			part.InputAudio = &types.InputAudioPart{Data: p.AudioData, Format: p.AudioFormat}
		}
		out.ContentParts = append(out.ContentParts, part)
	}
	for _, call := range m.ToolUse {
		out.ToolUse = append(out.ToolUse, types.ToolCall{
			ID: call.ID, Name: call.Name, Arguments: call.Arguments,
		})
	}
	for _, res := range m.ToolResults {
		out.ToolResults = append(out.ToolResults, types.ToolResult{
			ToolUseID: res.ToolUseID, Content: res.Content, IsError: res.IsError,
		})
	}
	return out
}

func cloneSingleMessage(m types.EyrieMessage) types.EyrieMessage {
	clones := cloneMessages([]types.EyrieMessage{m})
	return clones[0]
}
