package cmd

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Streaming and prompt command functions extracted from chat.go

func (m *chatModel) startPromptCommand(display, prompt string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "user", content: display})
	m.session.AddUser(prompt)
	m.waiting = true
	m.viewDirty = true
	m.partial.Reset()
	m.startStream()
	return m, nil
}

func (m *chatModel) startStream() {
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
		for ev := range ch {
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
			case "error":
				ref.Send(streamErrMsg{err: fmt.Errorf("%s", ev.Content)})
				return
			case "done":
				ref.Send(streamDoneMsg{})
				return
			}
		}
		ref.Send(streamDoneMsg{})
	}()
}
