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

func TestBuildWelcomeMessage_InlineShowsGuidance(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, nil)
	// Assert on copy present in both the needs-setup and ready branches so the
	// test is deterministic regardless of the host machine's /config state.
	for _, want := range []string{
		"/help",
		"/config",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("inline welcome missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildWelcomeMessage_ShortTerminalUsesCompactCopy(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 72, 20, nil)
	if strings.Contains(out, "PgUp/Dn scroll chat") || strings.Contains(out, "for new session") {
		t.Fatalf("compact welcome should drop verbose descriptions, got:\n%s", out)
	}
	if !strings.Contains(out, "/help") || !strings.Contains(out, "/config") {
		t.Fatalf("compact welcome should keep core commands, got:\n%s", out)
	}
}
