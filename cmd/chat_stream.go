package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Streaming and prompt command functions extracted from chat.go

func (m *chatModel) startPromptCommand(display, prompt string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, displayMsg{role: "user", content: display})
	m.session.AddUser(prompt)
	m.waiting = true
	m.partial.Reset()
	m.startStream()
	return m, nil
}

func (m *chatModel) startStream() {
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
			case "usage":
				// Usage events are only emitted in stream-json print mode
				// TUI mode ignores them since cost is tracked separately
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

// renderGlimmerVerb renders the spinner verb with a sweeping 3-color wave
func renderGlimmerVerb(verb string, glimmerPos int) string {
	if len(verb) == 0 {
		return ""
	}
	// 3 colors: vivid orange, bright orange, warm amber
	colors := []string{"255;94;14", "255;140;50", "255;180;80"}
	runes := []rune(verb)

	var b strings.Builder
	for i, r := range runes {
		// Wave: each char picks a color based on position + time, cycling 1→2→3→2→1→2→3...
		dist := abs((i + glimmerPos) % 5 - 2)
		if dist == 0 {
			b.WriteString("\033[1;38;2;" + colors[0] + "m" + string(r))
		} else if dist == 1 {
			b.WriteString("\033[1;38;2;" + colors[1] + "m" + string(r))
		} else {
			b.WriteString("\033[1;38;2;" + colors[2] + "m" + string(r))
		}
	}
	b.WriteString("\033[0m")
	return b.String()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
