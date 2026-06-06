package repomap

import (
	"strings"
)

// Render produces the compact, token-budgeted map string. Files are listed in
// descending rank order; the most-referenced files (and their symbols) appear
// first and are most likely to survive truncation under a tight budget.
func (g *Graph) Render(opts Options) string {
	budget := opts.Budget
	if budget <= 0 {
		budget = DefaultBudget
	}
	maxSyms := opts.MaxSymbolsPerFile
	if maxSyms <= 0 {
		maxSyms = DefaultMaxSymbolsPerFile
	}

	var b strings.Builder
	used := 0
	for _, node := range g.rankedFiles() {
		block := renderFile(node, maxSyms, opts.ExportedOnly)
		if block == "" {
			continue
		}
		cost := EstimateTokens(block)
		if used+cost > budget {
			// If nothing has been emitted yet, emit at least the header line of
			// the top file so the map is never empty for a positive budget.
			if used == 0 {
				header := node.Path + "\n"
				if EstimateTokens(header) <= budget {
					b.WriteString(header)
				}
			}
			break
		}
		b.WriteString(block)
		used += cost
	}
	return b.String()
}

// renderFile formats a single file block: a path header followed by indented
// symbol lines.
func renderFile(node *FileNode, maxSyms int, exportedOnly bool) string {
	var b strings.Builder
	b.WriteString(node.Path)
	b.WriteString("\n")

	count := 0
	for _, s := range node.Symbols {
		if exportedOnly && !s.Exported {
			continue
		}
		if count >= maxSyms {
			b.WriteString("  …\n")
			break
		}
		b.WriteString("  ")
		b.WriteString(s.Kind)
		b.WriteString(" ")
		b.WriteString(s.Name)
		b.WriteString("\n")
		count++
	}
	return b.String()
}

// EstimateTokens approximates the token count of s. It uses a simple,
// deterministic word/character heuristic (~4 chars per token, with a minimum of
// one token per whitespace-delimited field) that is adequate for budgeting and
// avoids any model-specific tokenizer dependency.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	fields := strings.Fields(s)
	byChars := (len(s) + 3) / 4
	if len(fields) > byChars {
		return len(fields)
	}
	return byChars
}
