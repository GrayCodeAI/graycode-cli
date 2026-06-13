package cmd

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// Streaming and prompt command functions extracted from chat.go

func (m *chatModel) startPromptCommand(display, prompt string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "user", content: display})
	m.session.AddUser(prompt)
	m.turnSawThinking = false
	m.turnHadAssistantOutput = false
	m.turnHadToolActivity = false
	m.waiting = true
	m.viewDirty = true
	m.partial.Reset()
	m.startStream()
	return m, nil
}

// dispatchStreamEvent maps one engine event to TUI messages. Returns true when
// the pump should stop (error or done).
func dispatchStreamEvent(ref *progRef, ev engine.StreamEvent) bool {
	switch ev.Type {
	case "content":
		ref.Send(streamChunkMsg(ev.Content))
	case "thinking":
		ref.Send(thinkingMsg(ev.Content))
	case "tool_use":
		ref.Send(toolUseMsg{name: ev.ToolName, id: ev.ToolID})
	case "tool_result":
		ref.Send(toolResultMsg{name: ev.ToolName, content: ev.Content})
	case "blast_radius":
		ref.Send(blastRadiusMsg{message: ev.Content})
	case "compact_start":
		ref.Send(compactStartMsg{})
	case "compact":
		ref.Send(compactMsg{
			strategy:     ev.Content,
			tokensBefore: ev.TokensBefore,
			tokensAfter:  ev.TokensAfter,
		})
	case "usage":
		if ev.Usage != nil {
			ref.Send(usageUpdateMsg{usage: ev.Usage})
		}
	case "retry":
		ref.Send(streamRetryMsg{content: ev.Content})
	case "error":
		ref.Send(streamErrMsg{err: fmt.Errorf("%s", ev.Content)})
		return true
	case "done":
		ref.Send(streamDoneMsg{})
		return true
	}
	return false
}

// pumpStreamEvents drains the engine channel into Bubble Tea messages.
func pumpStreamEvents(ref *progRef, ch <-chan engine.StreamEvent) {
	for ev := range ch {
		if dispatchStreamEvent(ref, ev) {
			return
		}
	}
	ref.Send(streamDoneMsg{})
}

func (m *chatModel) startStream() {
	m.streamCancelled = false
	m.syncSessionSelection()
	sess := m.session
	ref := m.ref
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go func() {
		ch, err := sess.Stream(ctx)
		if err != nil {
			ref.Send(streamErrMsg{err: err})
			return
		}
		pumpStreamEvents(ref, ch)
	}()
}
