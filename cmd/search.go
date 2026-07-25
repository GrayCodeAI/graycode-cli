package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/spf13/cobra"
)

var (
	searchLimit int
	searchJSON  bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across saved sessions",
	Long: `Full-text search across all saved hawk sessions.

Searches message content, tool results, and assistant responses.

Examples:
  hawk search "authentication"
  hawk search --limit 5 "database migration"
  hawk search "func main"`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "Maximum results to return")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output as JSON")
}

func runSearch(_ *cobra.Command, args []string) error {
	query := args[0]

	results, err := session.SearchSessions(query, searchLimit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No results for %q\n", query)
		return nil
	}

	if searchJSON {
		fmt.Print("[")
		for i, r := range results {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(`{"session_id":"%s","role":"%s","preview":"%s"}`,
				r.SessionID, r.Role, escapeJSON(r.Preview))
		}
		fmt.Println("]")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "SESSION\tROLE\tMATCH\n")
	for _, r := range results {
		preview := r.Preview
		if len(preview) > 80 {
			// Rune-safe truncation: never split a multibyte UTF-8 sequence.
			if runes := []rune(preview); len(runes) > 80 {
				preview = string(runes[:80]) + "..."
			}
		}
		preview = strings.ReplaceAll(preview, "\n", " ")
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", r.SessionID[:8], r.Role, preview)
	}
	return w.Flush()
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
