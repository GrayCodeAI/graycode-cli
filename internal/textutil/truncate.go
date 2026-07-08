// Package textutil provides shared, rune-safe string formatting helpers.
package textutil

// Truncate truncates s to at most max runes, appending "..." when content is
// dropped. It is rune-safe and never splits a multi-byte character.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
