package cmd

import "github.com/GrayCodeAI/hawk/internal/textutil"

// truncateWithEllipsis truncates s to at most max runes, appending "..." when
// content is dropped. Rune-safe: never splits a multi-byte character.
func truncateWithEllipsis(s string, max int) string {
	return textutil.Truncate(s, max)
}
