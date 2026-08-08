package cmd

import "strings"

// sanitizeDisplay neutralizes terminal escape sequences and control characters
// in untrusted content (tool results, model output, file contents, web fetches)
// before it reaches the TUI render path.
//
// Without this, a repo file or model response containing CSI/OSC sequences
// (e.g. "\x1b[2J" clear-screen, "\x1b]0;...\x07" title hijack, cursor moves)
// could forge permission prompts or corrupt the terminal session. Legitimate
// newlines and tabs are preserved; all other C0 controls (including \r, which
// is used for line-redraw tricks) and ESC sequences are removed.
//
// This strips raw escapes only — the lipgloss styling applied by the renderer
// happens AFTER sanitizing, so legitimate styling is unaffected.
func sanitizeDisplay(s string) string {
	if !strings.ContainsAny(s, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return s // fast path: nothing to scrub
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\t':
			b.WriteByte(c)
		case c == 0x1b: // ESC — consume the full escape sequence
			i = skipEscapeSequence(s, i)
		case c < 0x20 || c == 0x7f: // other C0 controls and DEL
			// drop
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// skipEscapeSequence returns the index of the last byte of the escape sequence
// starting at s[i] (where s[i] == 0x1b). Handles:
//
//   - CSI: ESC [ ... final-byte (0x40-0x7e)
//   - OSC: ESC ] ... ST (ESC \ or BEL)
//   - two-byte sequences: ESC <single char>
//   - lone ESC: returns i (the ESC itself is dropped by the caller)
func skipEscapeSequence(s string, i int) int {
	if i+1 >= len(s) {
		return i
	}
	switch s[i+1] {
	case '[': // CSI — consume until a final byte in 0x40-0x7e
		j := i + 2
		for j < len(s) {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j
			}
			j++
		}
		return len(s) - 1
	case ']': // OSC — consume until ST (ESC \) or BEL (0x07)
		j := i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				return j
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 1
			}
			j++
		}
		return len(s) - 1
	default: // two-byte ESC sequence (e.g. ESC c, ESC 7)
		return i + 1
	}
}
