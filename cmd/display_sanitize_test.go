package cmd

import (
	"strings"
	"testing"
)

func TestSanitizeDisplay_FastPath(t *testing.T) {
	// Plain content with no control chars must pass through unchanged.
	in := "hello world, normal content"
	if got := sanitizeDisplay(in); got != in {
		t.Errorf("sanitizeDisplay(plain) = %q, want unchanged", got)
	}
}

func TestSanitizeDisplay_StripsCSI(t *testing.T) {
	in := "before\x1b[2Jafter" // clear-screen sequence
	got := sanitizeDisplay(in)
	if got != "beforeafter" {
		t.Errorf("sanitizeDisplay(CSI) = %q, want %q", got, "beforeafter")
	}
}

func TestSanitizeDisplay_StripsOSC(t *testing.T) {
	// OSC title sequence terminated by BEL.
	got := sanitizeDisplay("a\x1b]0;evil-title\x07b")
	if got != "ab" {
		t.Errorf("sanitizeDisplay(OSC/BEL) = %q, want %q", got, "ab")
	}
	// OSC terminated by ESC \ (ST).
	got = sanitizeDisplay("a\x1b]0;evil\x1b\\b")
	if got != "ab" {
		t.Errorf("sanitizeDisplay(OSC/ST) = %q, want %q", got, "ab")
	}
}

func TestSanitizeDisplay_StripsC0Controls(t *testing.T) {
	// Carriage return (line-redraw trick) and other C0 controls removed.
	got := sanitizeDisplay("a\rb\x07c")
	if got != "abc" {
		t.Errorf("sanitizeDisplay(C0) = %q, want %q", got, "abc")
	}
	// Backspace and vertical tab.
	if got := sanitizeDisplay("x\by"); got != "xy" {
		t.Errorf("sanitizeDisplay(backspace) = %q, want %q", got, "xy")
	}
}

func TestSanitizeDisplay_PreservesNewlinesAndTabs(t *testing.T) {
	in := "line1\n\tline2\nline3"
	if got := sanitizeDisplay(in); got != in {
		t.Errorf("sanitizeDisplay(newlines/tabs) = %q, want %q", got, in)
	}
}

func TestSanitizeDisplay_TwoByteEscape(t *testing.T) {
	// ESC c (reset) and ESC 7 (save cursor) are two-byte sequences.
	if got := sanitizeDisplay("a\x1b7b"); got != "ab" {
		t.Errorf("sanitizeDisplay(two-byte ESC) = %q, want %q", got, "ab")
	}
}

func TestSanitizeDisplay_UnterminatedEscape(t *testing.T) {
	// A dangling ESC at the end must be dropped without panic.
	if got := sanitizeDisplay("abc\x1b"); got != "abc" {
		t.Errorf("sanitizeDisplay(lone ESC) = %q, want %q", got, "abc")
	}
	// Dangling CSI without a final byte.
	if got := sanitizeDisplay("abc\x1b["); got != "abc" {
		t.Errorf("sanitizeDisplay(dangling CSI) = %q, want %q", got, "abc")
	}
}

func TestSanitizeDisplay_UnicodePreserved(t *testing.T) {
	in := "héllo — 世界 ✓"
	if got := sanitizeDisplay(in); got != in {
		t.Errorf("sanitizeDisplay(unicode) = %q, want %q", got, in)
	}
}

func TestSanitizeDisplay_InjectionsInRealisticContent(t *testing.T) {
	// A malicious file content trying to hide the cursor and forge text.
	in := "cat output\x1b[?25l\x1b[1;1H[ALLOW] fake permission prompt\x07"
	got := sanitizeDisplay(in)
	if strings.Contains(got, "\x1b") {
		t.Errorf("sanitizeDisplay left an escape byte in: %q", got)
	}
	if strings.Contains(got, "\x07") {
		t.Errorf("sanitizeDisplay left a BEL byte in: %q", got)
	}
	// Legible text survives.
	if !strings.Contains(got, "cat output") {
		t.Errorf("sanitizeDisplay lost legit content: %q", got)
	}
}
