package diff

import (
	"fmt"
	"strings"
)

// SearchReplaceBlock is a single SEARCH/REPLACE pair extracted from LLM output.
//
// The on-wire format used by LLMs (e.g. Kimi-Dev, aider) is:
//
//	<<<<<<< SEARCH
//	old content to find
//	=======
//	new content to replace with
//	>>>>>>> REPLACE
type SearchReplaceBlock struct {
	Search  string
	Replace string
}

const (
	srSearchStart = "<<<<<<< SEARCH"
	srDivider     = "======="
	srReplaceEnd  = ">>>>>>> REPLACE"
)

// ParseSearchReplace extracts all SEARCH/REPLACE blocks from LLM output text.
// Text outside blocks (prose, code fences, explanations) is silently ignored.
// Incomplete or malformed blocks are skipped.
func ParseSearchReplace(text string) []SearchReplaceBlock {
	var blocks []SearchReplaceBlock

	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) {
		// Scan forward for the opening marker.
		if strings.TrimSpace(lines[i]) != srSearchStart {
			i++
			continue
		}
		i++ // skip the SEARCH marker line

		// Collect SEARCH content until the divider.
		var searchLines []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != srDivider {
			searchLines = append(searchLines, lines[i])
			i++
		}
		if i >= len(lines) {
			// Reached EOF without finding divider: malformed block, skip.
			break
		}
		i++ // skip the "=======" line

		// Collect REPLACE content until the closing marker.
		var replaceLines []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != srReplaceEnd {
			replaceLines = append(replaceLines, lines[i])
			i++
		}
		if i >= len(lines) {
			// Reached EOF without closing marker: malformed block, skip.
			break
		}
		i++ // skip the ">>>>>>> REPLACE" line

		blocks = append(blocks, SearchReplaceBlock{
			Search:  strings.Join(searchLines, "\n"),
			Replace: strings.Join(replaceLines, "\n"),
		})
	}

	return blocks
}

// ApplySearchReplace applies a sequence of SearchReplaceBlocks to content in order.
//
// Rules:
//   - Each block's Search string must appear exactly once in the current content
//     (after all prior substitutions have been applied). If it is not found at all,
//     ApplySearchReplace returns an error. If it appears more than once, the
//     replacement is ambiguous and an error is returned.
//   - An empty Replace string is valid and causes the matched text to be deleted.
//   - Blocks are applied left-to-right; each successive block operates on the
//     result of the previous one.
func ApplySearchReplace(content string, blocks []SearchReplaceBlock) (string, error) {
	current := content
	for i, b := range blocks {
		count := strings.Count(current, b.Search)
		switch {
		case count == 0:
			return "", fmt.Errorf(
				"search/replace block %d: SEARCH string not found in content\nSEARCH:\n%s",
				i+1, b.Search,
			)
		case count > 1:
			return "", fmt.Errorf(
				"search/replace block %d: SEARCH string is ambiguous (%d occurrences)\nSEARCH:\n%s",
				i+1, count, b.Search,
			)
		}
		current = strings.Replace(current, b.Search, b.Replace, 1)
	}
	return current, nil
}
