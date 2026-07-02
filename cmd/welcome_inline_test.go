package cmd

import (
	"strings"
	"testing"

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
