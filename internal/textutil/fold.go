package textutil

import (
	"unicode"
	"unicode/utf8"
)

// IndexFold returns the byte offset and byte length of the first
// case-insensitive match of substr in s, comparing rune-by-rune via
// unicode.ToLower. It returns (-1, 0) when substr is empty or not found.
//
// Unlike strings.Index on a lowercased copy of s, the returned span refers to
// the original string and always falls on rune boundaries, so callers can slice
// s at [idx, idx+matchLen) without ever splitting a multi-byte rune — even for
// runes whose case-folded form has a different encoded byte length (e.g. İ, ẞ),
// which is precisely where lowercasing the haystack and reusing the offset on
// the original corrupts output or panics.
func IndexFold(s, substr string) (idx, matchLen int) {
	sub := []rune(substr)
	if len(sub) == 0 {
		return -1, 0
	}
	for start := 0; start < len(s); {
		if end, ok := matchFoldPrefix(s, start, sub); ok {
			return start, end - start
		}
		_, size := utf8.DecodeRuneInString(s[start:])
		start += size
	}
	return -1, 0
}

// matchFoldPrefix reports whether s[start:] begins with sub under rune-wise
// case-insensitive comparison, and if so returns the byte offset just past the
// matched region in s.
func matchFoldPrefix(s string, start int, sub []rune) (end int, ok bool) {
	i := start
	for _, want := range sub {
		if i >= len(s) {
			return 0, false
		}
		got, size := utf8.DecodeRuneInString(s[i:])
		if unicode.ToLower(got) != unicode.ToLower(want) {
			return 0, false
		}
		i += size
	}
	return i, true
}
