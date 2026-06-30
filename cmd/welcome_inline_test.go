package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestBuildWelcomeMessage_InlineShowsSetupGuidance(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, nil)
	if !strings.Contains(out, "v") {
		t.Fatalf("inline welcome should show version, got:\n%s", out)
	}
	if strings.Contains(out, "WELCOME TO") {
		t.Fatalf("inline welcome should not render the old gate banner, got:\n%s", out)
	}
}

func TestBuildWelcomeMessage_InlineShowsStarterPrompts(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, nil)
	// Assert on copy present in both the needs-setup and ready branches so the
	// test is deterministic regardless of the host machine's /config state.
	for _, want := range []string{
		"explain this repo",
		"fix the failing test",
		"/help",
		"/config",
		"/permissions",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("inline welcome missing %q in:\n%s", want, out)
		}
	}
}

func TestFixedWelcomeLineCount_ReservesInlineHeaderSpace(t *testing.T) {
	ta := textarea.New()
	ta.SetWidth(96)
	ta.SetHeight(1)
	m := chatModel{
		width:        100,
		height:       30,
		welcomeCache: buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, nil),
		input:        ta,
		viewport:     viewport.New(100, 8),
	}
	if got := m.fixedWelcomeLineCount(); got == 0 {
		t.Fatal("expected inline welcome to reserve layout space")
	}
}

func TestBuildWelcomeMessage_ShortTerminalUsesCompactCopy(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 72, 20, nil)
	if strings.Contains(out, "PgUp/Dn scroll chat") {
		t.Fatalf("compact welcome should drop the long shortcuts row, got:\n%s", out)
	}
	if !strings.Contains(out, "explain this repo") {
		t.Fatalf("compact welcome should keep starter prompt guidance, got:\n%s", out)
	}
	if !strings.Contains(out, "Host mode runs commands locally") {
		t.Fatalf("compact welcome should explain host mode, got:\n%s", out)
	}
}
