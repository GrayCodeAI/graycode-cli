package cmd

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

func TestChatScrollbarVisible_WhenContentOverflows(t *testing.T) {
	m := chatModel{
		viewport:     viewportWithSize(80, 20),
		contentLines: 200,
	}
	if !m.chatScrollbarVisible() {
		t.Fatal("expected scrollable when content exceeds viewport height")
	}
}

func TestChatScrollbarVisible_NotWhenContentFits(t *testing.T) {
	m := chatModel{
		viewport:     viewportWithSize(80, 20),
		contentLines: 10,
	}
	if m.chatScrollbarVisible() {
		t.Fatal("expected not scrollable when content fits")
	}
}

func TestChatViewportWidth_FullTerminalWidth(t *testing.T) {
	m := chatModel{
		viewport:     viewportWithSize(80, 20),
		contentLines: 200,
		width:        80,
	}
	if w := m.chatViewportWidth(80); w != 80 {
		t.Fatalf("expected full width 80, got %d", w)
	}
}

func viewportWithSize(width, height int) viewport.Model {
	return viewport.New(width, height)
}