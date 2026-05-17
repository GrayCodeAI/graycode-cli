package engine

import "strings"

// EditStrategy selects the best edit format based on the task.
type EditStrategy int

const (
	EditWholeFile     EditStrategy = iota // replace entire file (small files <100 lines)
	EditSearchReplace                     // precise search-and-replace blocks
	EditDiff                              // unified diff format
	EditAppend                            // append to end of file
	EditInsert                            // insert at specific location
)

// SelectEditStrategy picks the optimal edit approach based on file size and change type.
func SelectEditStrategy(fileLines int, changeDescription string) EditStrategy {
	lower := strings.ToLower(changeDescription)

	// Small files → whole file replacement (simpler, less error-prone)
	if fileLines < 50 {
		return EditWholeFile
	}

	// Append patterns
	if strings.Contains(lower, "add") && (strings.Contains(lower, "end") || strings.Contains(lower, "bottom") || strings.Contains(lower, "new function") || strings.Contains(lower, "new method")) {
		return EditAppend
	}

	// Insert patterns
	if strings.Contains(lower, "insert") || strings.Contains(lower, "before") || strings.Contains(lower, "after line") {
		return EditInsert
	}

	// Large files → search-replace (precise, minimal diff)
	if fileLines > 200 {
		return EditSearchReplace
	}

	// Default: search-replace for medium files
	return EditSearchReplace
}

// EditStrategyPrompt returns instructions for the LLM based on the selected strategy.
func EditStrategyPrompt(strategy EditStrategy) string {
	switch strategy {
	case EditWholeFile:
		return "Return the COMPLETE updated file content. The file is small enough to replace entirely."
	case EditSearchReplace:
		return `Use SEARCH/REPLACE blocks to make changes:
<<<<<<< SEARCH
exact lines to find
=======
replacement lines
>>>>>>> REPLACE
Each block must match exactly. Use multiple blocks for multiple changes.`
	case EditDiff:
		return "Return changes as a unified diff (--- a/file, +++ b/file, @@ hunks)."
	case EditAppend:
		return "Return ONLY the new code to append at the end of the file."
	case EditInsert:
		return "Specify the line number and the code to insert. Format: INSERT AFTER LINE N:"
	default:
		return ""
	}
}
