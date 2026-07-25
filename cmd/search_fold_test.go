package cmd

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
		wantMatch string // s[idx:idx+len] when found
	}{
		{"ascii exact", "Hello World", "World", 6, 5, "World"},
		{"ascii case-insensitive", "Hello World", "world", 6, 5, "World"},
		{"ascii upper query", "hello", "HELLO", 0, 5, "hello"},
		{"empty substr", "Hello", "", -1, 0, ""},
		{"no match", "Hello", "xyz", -1, 0, ""},
		{"substr longer than s", "Hi", "Hello", -1, 0, ""},
		{"match at start", "target practice", "target", 0, 6, "target"},
		{"match at end", "practice target", "target", 9, 6, "target"},
		{"cjk no case", "日本語テスト", "テスト", 9, 9, "テスト"},
		{"cjk mixed with ascii", "hello世界world", "世界", 5, 6, "世界"},
		// İ (U+0130) lowercases to a form with a different byte length. The old
		// byte-offset approach shifted the match index past a rune boundary and
		// panicked slicing s[3:9] on an 8-byte string; indexFold must return the
		// rune-aligned span of the original string instead.
		{"fold length change İ", "İtarget", "target", 2, 6, "target"},
		{"fold ẞ to ß", "ẞtraße", "ßtr", 0, 5, "ẞtr"},
		{"accented", "café au lait", "CAFÉ", 0, 5, "café"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, matchLen := indexFold(tt.s, tt.substr)
			if idx != tt.wantIdx || matchLen != tt.wantLen {
				t.Fatalf("indexFold(%q, %q) = (%d, %d), want (%d, %d)",
					tt.s, tt.substr, idx, matchLen, tt.wantIdx, tt.wantLen)
			}
			if idx >= 0 {
				if idx+matchLen > len(tt.s) {
					t.Fatalf("span out of bounds: idx=%d len=%d > len(s)=%d", idx, matchLen, len(tt.s))
				}
				got := tt.s[idx : idx+matchLen]
				if got != tt.wantMatch {
					t.Errorf("matched span = %q, want %q", got, tt.wantMatch)
				}
				if !utf8.ValidString(got) {
					t.Errorf("matched span is not valid UTF-8: %q", got)
				}
			}
		})
	}
}

// TestIndexFold_SpansAlwaysValidUTF8 asserts the core safety invariant: for a
// range of multi-byte inputs, any reported match span lies on rune boundaries
// and is valid UTF-8, so callers can slice without splitting a rune.
func TestIndexFold_SpansAlwaysValidUTF8(t *testing.T) {
	inputs := []string{
		"İtarget", "aİb target", "日本語target日本語", "café TARGET café",
		"ẞtraße ßtr", "mixed 世界 target 世界", "İİİtarget",
	}
	queries := []string{"target", "TARGET", "ßtr", "世界", "café", "İ"}
	for _, s := range inputs {
		for _, q := range queries {
			idx, matchLen := indexFold(s, q)
			if idx < 0 {
				continue
			}
			if idx+matchLen > len(s) {
				t.Fatalf("indexFold(%q,%q): span [%d:%d] out of bounds (len %d)", s, q, idx, idx+matchLen, len(s))
			}
			span := s[idx : idx+matchLen]
			if !utf8.ValidString(span) {
				t.Fatalf("indexFold(%q,%q): span %q is not valid UTF-8", s, q, span)
			}
		}
	}
}

func TestHighlightMatch_MultibyteSafe(t *testing.T) {
	tests := []struct {
		entry string
		query string
	}{
		{"İtarget", "target"}, // old byte-offset code panicked here (slice out of range)
		{"日本語テスト", "テスト"},
		{"café au lait", "CAFÉ"},
		{"hello 世界 world", "世界"},
		{"plain ascii", "ascii"},
	}
	for _, tt := range tests {
		t.Run(tt.entry+"|"+tt.query, func(t *testing.T) {
			out := highlightMatch(tt.entry, tt.query, 200)
			if !utf8.ValidString(out) {
				t.Fatalf("highlightMatch output is not valid UTF-8: %q", out)
			}
			// Highlighting only wraps the match in ANSI; stripping it must recover
			// the original entry (prefixed with the two-space indent).
			if plain := stripAnsi(out); plain != "  "+tt.entry {
				t.Errorf("stripAnsi(output) = %q, want %q", plain, "  "+tt.entry)
			}
		})
	}
}
