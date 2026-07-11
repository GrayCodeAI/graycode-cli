package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

			tea "charm.land/bubbletea/v2"

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

func dispatchStreamEventWithFlush(ref *progRef, ev engine.StreamEvent, flush func()) bool {
	if ev.Type != "content" && flush != nil {
		flush()
	}
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

const streamChunkCoalesceInterval = 16 * time.Millisecond

// pumpStreamEvents drains the engine channel into Bubble Tea messages.
func pumpStreamEvents(ref *progRef, ch <-chan engine.StreamEvent) {
	var buf strings.Builder
	firstContent := true
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		ref.Send(streamChunkMsg(buf.String()))
		buf.Reset()
	}
	defer flush()

	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	startTimer := func() {
		if timer != nil {
			return
		}
		timer = time.NewTimer(streamChunkCoalesceInterval)
		timerC = timer.C
	}
	for {
		select {
		case <-timerC:
			flush()
			timer = nil
			timerC = nil
		case ev, ok := <-ch:
			if !ok {
				stopTimer()
				flush()
				ref.Send(streamDoneMsg{})
				return
			}
			if ev.Type == "content" {
				if firstContent {
					firstContent = false
					ref.Send(streamChunkMsg(ev.Content))
					continue
				}
				buf.WriteString(ev.Content)
				if shouldFlushStreamChunkBuffer(buf.String()) {
					stopTimer()
					flush()
				} else {
					startTimer()
				}
				continue
			}
			stopTimer()
			if dispatchStreamEventWithFlush(ref, ev, flush) {
				return
			}
		}
	}
}

func shouldFlushStreamChunkBuffer(s string) bool {
	if len(s) >= 512 {
		return true
	}
	return strings.ContainsAny(s, "\n.!?;:")
}

func (m *chatModel) startStream() {
	m.turnEstimatedOutputRunes = 0
	if m.testStreamStarter != nil {
		m.testStreamStarter()
		return
	}
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
