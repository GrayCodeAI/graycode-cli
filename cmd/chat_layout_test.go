package cmd

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
)

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
	m.viewDirty = true
	m.updateViewportContent()
	got := m.View()
	if !strings.Contains(got, "HAWK LOGO") {
		t.Fatalf("welcome should be pinned at top, got prefix: %q", got[:min(40, len(got))])
	}
	if !strings.Contains(got, "Host mode:") && !strings.Contains(got, "Container:") {
		t.Fatalf("footer should be present at bottom")
	}
}

func TestPrimeInitialViewportContent_RendersWelcomeBeforeFirstFrame(t *testing.T) {
	m := chatModel{
		height:       24,
		width:        80,
		welcomeCache: "HAWK LOGO\nv0.1.0",
		input:        textarea.New(),
		viewport:     viewport.New(80, 8),
		ghostText:    NewGhostText(),
	}
	m = m.withSyncedLayout()

	if strings.Contains(m.viewport.View(), "HAWK LOGO") {
		t.Fatal("expected empty initial viewport before priming")
	}

	m.primeInitialViewportContent()

	if !strings.Contains(m.viewport.View(), "HAWK LOGO") {
		t.Fatalf("expected primed viewport to include welcome content, got %q", m.viewport.View())
	}
}
