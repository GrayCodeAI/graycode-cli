package cmd

import (
	"fmt"
	"strings"

			tea "charm.land/bubbletea/v2"
)

type copyMode int

const (
	copyModeSmart copyMode = iota
	copyModeAll
	copyModeLast
	copyModeInput
	copyModeAssistant
)

// isCopyToClipboardKey matches keyboard shortcuts for copy-to-clipboard without
// conflicting with Ctrl+C (cancel). Works across common terminal encodings.
func isCopyToClipboardKey(msg tea.KeyMsg) bool {
	switch strings.ToLower(msg.String()) {
	case "alt+c", "ctrl+shift+c", "ctrl+alt+c", "meta+c":
		return true
	}
	return false
}

func (m chatModel) inputDraftForCopy() string {
	return strings.TrimSpace(m.input.Value())
}

func (m chatModel) copyableTranscript() string {
	partial := ""
	if m.partial != nil {
		partial = strings.TrimSpace(m.partial.String())
	}
	transcript := plainTranscript(m.messages, partial)
	if draft := m.inputDraftForCopy(); draft != "" {
		if transcript != "" {
			transcript += "\n\n"
		}
		transcript += "Draft: " + draft
	}
	return transcript
}

func (m chatModel) lastMessageContent() (string, bool) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		line, ok := plainTranscriptLine(m.messages[i])
		if ok {
			return line, true
		}
	}
	return "", false
}

func (m chatModel) lastAssistantContent() (string, bool) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "assistant" && strings.TrimSpace(m.messages[i].content) != "" {
			return m.messages[i].content, true
		}
	}
	return "", false
}

func (m chatModel) lastCopyableContent() (string, bool) {
	if content, ok := m.lastAssistantContent(); ok {
		return content, true
	}
	if transcript := m.copyableTranscript(); transcript != "" {
		return transcript, true
	}
	return "", false
}

func (m chatModel) smartCopyContent() (content, label string, ok bool) {
	if m.uiFocus == focusPrompt && !m.configOpen && !m.useConfigInput {
		if draft := m.inputDraftForCopy(); draft != "" {
			return draft, "input", true
		}
	}
	if content, ok := m.lastCopyableContent(); ok {
		return content, "chat", true
	}
	return "", "", false
}

func (m chatModel) copyContent(mode copyMode) (content, label string, ok bool) {
	switch mode {
	case copyModeInput:
		if draft := m.inputDraftForCopy(); draft != "" {
			return draft, "input", true
		}
	case copyModeAll:
		if transcript := m.copyableTranscript(); transcript != "" {
			return transcript, "chat transcript", true
		}
	case copyModeLast:
		if line, ok := m.lastMessageContent(); ok {
			return line, "last message", true
		}
	case copyModeAssistant:
		if content, ok := m.lastAssistantContent(); ok {
			return content, "assistant reply", true
		}
	case copyModeSmart:
		return m.smartCopyContent()
	}
	return "", "", false
}

func (m *chatModel) appendCopyResult(content, label string, err error) {
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Failed to copy: " + err.Error()})
		return
	}
	if label == "" {
		label = "content"
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Copied %s to clipboard.", label)})
	m.viewDirty = true
}

func (m *chatModel) handleCopyCommand(parts []string) (tea.Model, tea.Cmd) {
	mode := copyModeSmart
	if len(parts) > 1 {
		switch strings.ToLower(parts[1]) {
		case "all", "chat", "session", "transcript":
			mode = copyModeAll
		case "input", "prompt", "draft":
			mode = copyModeInput
		case "last", "message":
			mode = copyModeLast
		case "assistant", "reply", "response":
			mode = copyModeAssistant
		default:
			m.messages = append(m.messages, displayMsg{
				role: "system",
				content: "Usage: /copy [all|input|last|assistant]\n" +
					"  /copy           — smart copy (input draft or chat)\n" +
					"  /copy all       — full transcript\n" +
					"  /copy input     — prompt draft\n" +
					"  /copy last      — last message\n" +
					"  /copy assistant — last reply",
			})
			return m, nil
		}
	}
	content, label, ok := m.copyContent(mode)
	if !ok {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Nothing to copy."})
		return m, nil
	}
	m.appendCopyResult(content, label, copyToClipboard(content))
	return m, nil
}

func (m *chatModel) handleCopyShortcut() (tea.Model, tea.Cmd) {
	content, label, ok := m.smartCopyContent()
	if !ok {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Nothing to copy."})
		m.viewDirty = true
		return m, nil
	}
	m.appendCopyResult(content, label, copyToClipboard(content))
	return m, nil
}
