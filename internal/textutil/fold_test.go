package textutil

import (
	"testing"
	"unicode/utf8"
)

func TestIndexFold(t *testing.T) {
	tests := []struct {
		name      string
		s         string
		substr    string
		wantIdx   int
		wantLen   int
		wantMatch string
	}{
		{"ascii exact", "Hello World", "World", 6, 5, "World"},
		{"ascii case-insensitive", "Hello World", "world", 6, 5, "World"},
		{"empty substr", "Hello", "", -1, 0, ""},
		{"no match", "Hello", "xyz", -1, 0, ""},
		{"substr longer than s", "Hi", "Hello", -1, 0, ""},
		{"match at start", "target practice", "target", 0, 6, "target"},
		{"match at end", "practice target", "target", 9, 6, "target"},
		{"cjk no case", "日本語テスト", "テスト", 9, 9, "テスト"},
		// İ (U+0130) lowercases to a longer encoding. A byte offset taken from a
		// lowercased copy would overshoot the original string and panic; IndexFold
		// must return the rune-aligned span of the original instead.
		{"fold length change İ", "İtarget", "target", 2, 6, "target"},
		{"fold ẞ to ß", "ẞtraße", "ßtr", 0, 5, "ẞtr"},
		{"accented", "café au lait", "CAFÉ", 0, 5, "café"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, matchLen := IndexFold(tt.s, tt.substr)
			if idx != tt.wantIdx || matchLen != tt.wantLen {
				t.Fatalf("IndexFold(%q, %q) = (%d, %d), want (%d, %d)",
					tt.s, tt.substr, idx, matchLen, tt.wantIdx, tt.wantLen)
			}
			if idx >= 0 {
				if idx+matchLen > len(tt.s) {
					t.Fatalf("span out of bounds: [%d:%d] > len %d", idx, idx+matchLen, len(tt.s))
				}
				if got := tt.s[idx : idx+matchLen]; got != tt.wantMatch {
					t.Errorf("matched span = %q, want %q", got, tt.wantMatch)
				}
			}
		})
	}
}

// TestIndexFold_SpansAlwaysValidUTF8 asserts the core safety invariant: any
// reported span lies on rune boundaries of the original string.
func TestIndexFold_SpansAlwaysValidUTF8(t *testing.T) {
	inputs := []string{"İtarget", "aİb target", "日本語target日本語", "café TARGET café", "İİİtarget"}
	queries := []string{"target", "TARGET", "café", "İ"}
	for _, s := range inputs {
		for _, q := range queries {
			idx, matchLen := IndexFold(s, q)
			if idx < 0 {
				continue
			}
			if idx+matchLen > len(s) {
				t.Fatalf("IndexFold(%q,%q): span [%d:%d] out of bounds (len %d)", s, q, idx, idx+matchLen, len(s))
			}
			if span := s[idx : idx+matchLen]; !utf8.ValidString(span) {
				t.Fatalf("IndexFold(%q,%q): span %q is not valid UTF-8", s, q, span)
			}
		}
	}
}
