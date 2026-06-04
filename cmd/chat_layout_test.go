package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

func TestFixedWelcomeLineCount_ReservesChatSpace(t *testing.T) {
	m := chatModel{
		height:       30,
		width:        80,
		welcomeCache: strings.Repeat("line\n", 25),
		input:        textarea.New(),
		viewport:     viewport.New(80, 10),
	}
	w := m.fixedWelcomeLineCount()
	bottom := m.chatBottomBarLines()
	if w+bottom+minChatViewportLines > m.height {
		t.Fatalf("welcome %d + bottom %d exceeds height %d", w, bottom, m.height)
	}
}

func TestView_PinsWelcomeAboveViewport(t *testing.T) {
	m := chatModel{
		height:       24,
		width:        80,
		welcomeCache: "HAWK LOGO\nv0.1.0",
		input:        textarea.New(),
		viewport:     viewport.New(80, 8),
		ghostText:    NewGhostText(),
	}
	m = m.withSyncedLayout()
	got := m.View()
	if !strings.HasPrefix(got, "HAWK LOGO") {
		t.Fatalf("welcome should be pinned at top, got prefix: %q", got[:min(40, len(got))])
	}
	if !strings.Contains(got, "Default") && !strings.Contains(got, "Container:") {
		t.Fatalf("footer should be present at bottom")
	}
}
