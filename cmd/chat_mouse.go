package cmd

import (
	"fmt"
	"os"
	"strings"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

func mouseEnabledFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("GRAYCODE_MOUSE"))
	if v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "off") {
		return false
	}
	return true
}

func envOverridesMouse() bool {
	v := strings.TrimSpace(os.Getenv("GRAYCODE_MOUSE"))
	return v != ""
}

// mouseEnabled reports whether the TUI should capture mouse events for chat wheel
// scroll. When false, the terminal handles click-drag selection natively (OpenCode
// "mouse": false). Priority: GRAYCODE_MOUSE env → runtime override → settings → default on.
func (m chatModel) mouseEnabled() bool {
	if envOverridesMouse() {
		return mouseEnabledFromEnv()
	}
	if m.mouseOverride != nil {
		return *m.mouseOverride
	}
	if m.settings.TuiMouse != nil {
		return *m.settings.TuiMouse
	}
	return true
}

func (m *chatModel) setMouseEnabled(enabled bool) {
	m.mouseOverride = &enabled
	m.settings.TuiMouse = &enabled
	syncTerminalMouse(enabled)
	*m = m.syncViewportMouseWheel()
	m.viewDirty = true
}

func (m *chatModel) handleMouseCommand(parts []string) {
	if len(parts) < 2 {
		state := "on"
		if !m.mouseEnabled() {
			state = "off"
		}
		source := "default"
		switch {
		case envOverridesMouse():
			source = "GRAYCODE_MOUSE env"
		case m.mouseOverride != nil:
			source = "session"
		case m.settings.TuiMouse != nil:
			source = "settings"
		}
		m.messages = append(m.messages, displayMsg{
			role: "system",
			content: fmt.Sprintf(
				"Mouse capture: %s (%s)\n"+
					"  /mouse off  — native click-drag copy (OpenCode-style)\n"+
					"  /mouse on   — chat wheel scroll\n"+
					"  Shift+drag also bypasses capture in iTerm2/Ghostty",
				state, source,
			),
		})
		return
	}

	if envOverridesMouse() {
		m.messages = append(m.messages, displayMsg{
			role: "system",
			content: "Mouse is controlled by GRAYCODE_MOUSE env in this session. " +
				"Unset it to use /mouse or settings.json tui_mouse.",
		})
		return
	}

	switch strings.ToLower(parts[1]) {
	case "on", "true", "1", "enable":
		m.setMouseEnabled(true)
		_ = graycodeconfig.SetGlobalSetting("tui_mouse", "true")
		m.messages = append(m.messages, displayMsg{role: "system", content: "Mouse capture on — chat wheel scroll enabled."})
	case "off", "false", "0", "disable":
		m.setMouseEnabled(false)
		_ = graycodeconfig.SetGlobalSetting("tui_mouse", "false")
		m.messages = append(m.messages, displayMsg{
			role:    "system",
			content: "Mouse capture off — use click-drag to select text. /copy and Ctrl+Shift+C still work.",
		})
	case "toggle":
		next := !m.mouseEnabled()
		m.setMouseEnabled(next)
		val := "false"
		msg := "Mouse capture off — native click-drag copy enabled."
		if next {
			val = "true"
			msg = "Mouse capture on — chat wheel scroll enabled."
		}
		_ = graycodeconfig.SetGlobalSetting("tui_mouse", val)
		m.messages = append(m.messages, displayMsg{role: "system", content: msg})
	default:
		m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /mouse [on|off|toggle]"})
	}
}
