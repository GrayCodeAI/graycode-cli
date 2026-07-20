package icons

import (
	"testing"
	"unicode/utf8"
)

func TestRegistry_AllNamesHaveBothGlyphs(t *testing.T) {
	for _, n := range Names() {
		nerd, ascii, ok := lookup(n)
		if !ok {
			t.Fatalf("name %q not in registry", n)
		}
		if nerd == "" {
			t.Errorf("name %q: Nerd glyph empty", n)
		}
		if ascii == "" {
			t.Errorf("name %q: ASCII glyph empty", n)
		}
	}
}

func TestGlyph_ModeNerd(t *testing.T) {
	SetMode(ModeNerd)
	t.Cleanup(func() { SetMode(ModeAuto) })
	if got := Glyph("chevron_right"); got != puaChevronRight {
		t.Errorf("chevron_right Nerd = %q, want %q", got, puaChevronRight)
	}
}

func TestGlyph_ModeASCII(t *testing.T) {
	SetMode(ModeASCII)
	t.Cleanup(func() { SetMode(ModeAuto) })
	if got := Glyph("chevron_right"); got != ">" {
		t.Errorf("chevron_right ASCII = %q, want %q", got, ">")
	}
}

func TestASCII_IgnoresMode(t *testing.T) {
	SetMode(ModeNerd)
	if got := ASCII("chevron_right"); got != ">" {
		t.Errorf("ASCII chevron_right (Nerd mode) = %q", got)
	}
	SetMode(ModeASCII)
	if got := ASCII("chevron_right"); got != ">" {
		t.Errorf("ASCII chevron_right (ASCII mode) = %q", got)
	}
}

func TestNerd_IgnoresMode(t *testing.T) {
	SetMode(ModeASCII)
	if got := Nerd("chevron_right"); got != puaChevronRight {
		t.Errorf("Nerd chevron_right (ASCII mode) = %q", got)
	}
}

func TestASCII_AllRunesAreASCII(t *testing.T) {
	for _, n := range Names() {
		_, ascii, _ := lookup(n)
		for i, r := range ascii {
			if r >= 0x80 {
				t.Errorf("ASCII fallback for %q has non-ASCII rune %U at byte %d: %q", n, r, i, ascii)
			}
		}
	}
}

// TestNerdGlyphs_AllInPrivateUseArea is the lock-down test. Every
// Nerd Font codepoint in the registry MUST be in the Unicode Private
// Use Area (U+E000–U+F8FF) — no emoji, no symbol block, no stray
// codepoints. This is the regression test that prevents future
// contributors from adding an emoji by accident.
func TestNerdGlyphs_AllInPrivateUseArea(t *testing.T) {
	for _, n := range Names() {
		nerd, _, _ := lookup(n)
		if !utf8.ValidString(nerd) {
			t.Errorf("name %q: Nerd glyph is not valid UTF-8: %x", n, nerd)
		}
		for _, r := range nerd {
			if r < 0xE000 || r > 0xF8FF {
				t.Errorf("name %q: Nerd glyph contains rune %U which is outside the PUA range U+E000–U+F8FF", n, r)
			}
		}
	}
}

func TestGlyph_UnknownReturnsEmpty(t *testing.T) {
	if got := Glyph("nope"); got != "" {
		t.Errorf("Glyph(\"nope\") = %q, want empty string", got)
	}
	if got := ASCII("nope"); got != "" {
		t.Errorf("ASCII(\"nope\") = %q, want empty string", got)
	}
	if got := Nerd("nope"); got != "" {
		t.Errorf("Nerd(\"nope\") = %q, want empty string", got)
	}
}

func TestTypedAccessors_AllReturnNonEmpty(t *testing.T) {
	for _, m := range []IconMode{ModeNerd, ModeASCII} {
		SetMode(m)
		for _, n := range Names() {
			if got := Glyph(n); got == "" {
				t.Errorf("mode=%s: Glyph(%q) = empty", m, n)
			}
		}
	}
}

func TestNames_StableOrder(t *testing.T) {
	// Stable across calls.
	first := Names()
	second := Names()
	if len(first) != len(second) {
		t.Fatalf("Names() changed length: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, second[i], first[i])
		}
	}
}
