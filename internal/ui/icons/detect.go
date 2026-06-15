package icons

import (
	"os"
	"strings"
	"sync/atomic"
)

// Mode selects which glyph set the icons package returns.
type IconMode int32

const (
	// ModeAuto is resolved lazily by Mode() on first call.
	ModeAuto IconMode = iota
	// ModeASCII forces every glyph to its ASCII fallback.
	ModeASCII
	// ModeNerd forces every glyph to its Nerd Font PUA codepoint.
	ModeNerd
)

func (m IconMode) String() string {
	switch m {
	case ModeNerd:
		return "nerd"
	case ModeASCII:
		return "ascii"
	default:
		return "auto"
	}
}

// modeVal is the resolved-or-overridden active mode. Atomic so the
// race detector stays quiet when tests call SetMode concurrently.
var modeVal atomic.Int32

func init() {
	modeVal.Store(int32(ModeAuto))
}

// Mode returns the active rendering mode. On first call, resolves
// ModeAuto from env vars. Subsequent calls return the cached value
// unless SetMode is used.
//
// Resolution is conservative: ASCII is the default; Nerd Font is
// enabled only when env markers explicitly indicate a patched terminal
// is in use AND stdout is a TTY.
//
// Precedence:
//  1. HAWK_ICONS=nerd|ascii → ModeNerd / ModeASCII
//  2. NO_COLOR set          → ModeASCII
//  3. !stdoutIsTTY()        → ModeASCII  (piped output stays clean)
//  4. TERM / TERM_PROGRAM / LC_TERMINAL matches a known Nerd-Font-friendly
//     terminal → ModeNerd
//  5. LANG / LC_ALL / LC_CTYPE contains "UTF-8" → ModeNerd
//  6. otherwise             → ModeASCII
func Mode() IconMode {
	m := IconMode(modeVal.Load())
	if m != ModeAuto {
		return m
	}
	resolved := resolveMode()
	modeVal.Store(int32(resolved))
	return resolved
}

// SetMode overrides detection. Pass ModeAuto to re-enable detection
// on the next Mode() call. Tests use this to pin behaviour.
func SetMode(m IconMode) {
	modeVal.Store(int32(m))
}

func resolveMode() IconMode {
	switch strings.ToLower(os.Getenv("HAWK_ICONS")) {
	case "nerd", "nerdfont", "pua":
		return ModeNerd
	case "ascii", "off", "no", "0":
		return ModeASCII
	}
	if os.Getenv("NO_COLOR") != "" {
		return ModeASCII
	}
	if !stdoutIsTTY() {
		return ModeASCII
	}
	term := strings.ToLower(os.Getenv("TERM"))
	program := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	locus := strings.ToLower(os.Getenv("LC_TERMINAL"))
	for _, t := range knownNerdTerm {
		if strings.Contains(term, t) || strings.Contains(program, t) || strings.Contains(locus, t) {
			return ModeNerd
		}
	}
	for _, v := range []string{os.Getenv("LC_ALL"), os.Getenv("LC_CTYPE"), os.Getenv("LANG")} {
		if v == "" {
			continue
		}
		low := strings.ToLower(v)
		if strings.Contains(low, "utf-8") || strings.Contains(low, "utf8") {
			return ModeNerd
		}
	}
	return ModeASCII
}

var knownNerdTerm = []string{
	"xterm-256color", "tmux-256color", "screen-256color",
	"alacritty", "wezterm", "kitty", "ghostty",
	"vscode", "hyper", "iterm", "apple_terminal",
}

// stdoutIsTTY is overridable from tests.
var stdoutIsTTY = func() bool {
	fi, _ := os.Stdout.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) != 0
}
