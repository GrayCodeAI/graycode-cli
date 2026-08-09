package icons

import (
	"testing"
)

func TestMode_HAWKIconsNerdOverrides(t *testing.T) {
	t.Setenv("HAWK_ICONS", "nerd")
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// Even with NO_COLOR and a dumb TERM, HAWK_ICONS=nerd wins.
	withInjectedTTY(t, true)
	SetMode(ModeAuto)
	if Mode() != ModeNerd {
		t.Errorf("HAWK_ICONS=nerd should win, got %s", Mode())
	}
}

func TestMode_HAWKIconsAsciiOverrides(t *testing.T) {
	t.Setenv("HAWK_ICONS", "ascii")
	t.Setenv("TERM", "xterm-256color")
	withInjectedTTY(t, true)
	SetMode(ModeAuto)
	if Mode() != ModeASCII {
		t.Errorf("HAWK_ICONS=ascii should win, got %s", Mode())
	}
}

func TestMode_NoColorForcesAscii(t *testing.T) {
	t.Setenv("HAWK_ICONS", "")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	withInjectedTTY(t, true)
	SetMode(ModeAuto)
	if Mode() != ModeASCII {
		t.Errorf("NO_COLOR should force ASCII, got %s", Mode())
	}
}

func TestMode_NonTTYForcesAscii(t *testing.T) {
	t.Setenv("HAWK_ICONS", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	withInjectedTTY(t, false)
	SetMode(ModeAuto)
	if Mode() != ModeASCII {
		t.Errorf("non-TTY stdout should force ASCII, got %s", Mode())
	}
}

func TestMode_TTYDefaultsToNerd(t *testing.T) {
	for _, term := range []string{"xterm-256color", "tmux-256color", "screen-256color", "alacritty", "wezterm", "kitty", "ghostty"} {
		t.Run(term, func(t *testing.T) {
			t.Setenv("HAWK_ICONS", "")
			t.Setenv("NO_COLOR", "")
			t.Setenv("TERM", term)
			t.Setenv("TERM_PROGRAM", "")
			t.Setenv("LC_TERMINAL", "")
			withInjectedTTY(t, true)
			SetMode(ModeAuto)
			if Mode() != ModeNerd {
				t.Errorf("interactive TERM=%s should default to Nerd, got %s", term, Mode())
			}
		})
	}
}

func TestMode_TERMProgramDoesNotAffectMode(t *testing.T) {
	for _, program := range []string{"iTerm.app", "WezTerm", "Ghostty", "vscode", "hyper"} {
		t.Run(program, func(t *testing.T) {
			t.Setenv("HAWK_ICONS", "")
			t.Setenv("NO_COLOR", "")
			t.Setenv("TERM", "dumb")
			t.Setenv("LC_TERMINAL", "")
			t.Setenv("TERM_PROGRAM", program)
			withInjectedTTY(t, true)
			SetMode(ModeAuto)
			if Mode() != ModeNerd {
				t.Errorf("TERM_PROGRAM=%s should not change interactive Nerd default, got %s", program, Mode())
			}
		})
	}
}

func TestMode_UTF8LocaleDoesNotAffectMode(t *testing.T) {
	t.Setenv("HAWK_ICONS", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "en_US.UTF-8")
	withInjectedTTY(t, true)
	SetMode(ModeAuto)
	if Mode() != ModeNerd {
		t.Errorf("UTF-8 locale should not change interactive Nerd default, got %s", Mode())
	}
}

func TestMode_InteractiveTTYDefaultsToNerd(t *testing.T) {
	t.Setenv("HAWK_ICONS", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	withInjectedTTY(t, true)
	SetMode(ModeAuto)
	if Mode() != ModeNerd {
		t.Errorf("interactive dumb TERM should still default to Nerd, got %s", Mode())
	}
}

func TestSetMode_OverridesCaching(t *testing.T) {
	t.Setenv("HAWK_ICONS", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	withInjectedTTY(t, true)
	SetMode(ModeASCII)
	if Mode() != ModeASCII {
		t.Errorf("SetMode(ModeASCII) did not take effect")
	}
	SetMode(ModeNerd)
	if Mode() != ModeNerd {
		t.Errorf("SetMode(ModeNerd) did not take effect")
	}
	SetMode(ModeAuto)
	if Mode() != ModeNerd {
		t.Errorf("SetMode(ModeAuto) did not re-detect (TERM=xterm-256color expects Nerd), got %s", Mode())
	}
}

// withInjectedTTY swaps the package-private stdoutIsTTY predicate for
// the duration of the test. Restores on cleanup.
func withInjectedTTY(t *testing.T, isTTY bool) {
	t.Helper()
	prev := stdoutIsTTY
	stdoutIsTTY = func() bool { return isTTY }
	t.Cleanup(func() { stdoutIsTTY = prev })
}
