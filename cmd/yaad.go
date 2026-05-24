package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/spf13/cobra"
)

var yaadLimit int

var yaadCmd = &cobra.Command{
	Use:   "yaad",
	Short: "Yaad persistent memory graph",
	Long:  "Inspect and search yaad graph memory. Hawk embeds yaad at ~/.yaad/data/yaad.db — no separate daemon required.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if yaadLimit < 1 {
			return fmt.Errorf("--limit must be at least 1")
		}
		cmd.Println(memory.FormatYaadDetail(yaadLimit))
		return nil
	},
}

var yaadSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search yaad memories by keyword",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if yaadLimit < 1 {
			return fmt.Errorf("--limit must be at least 1")
		}
		query := strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			return fmt.Errorf("search query required")
		}
		cmd.Println(memory.FormatYaadSearch(query, yaadLimit))
		return nil
	},
}

func init() {
	yaadCmd.PersistentFlags().IntVar(&yaadLimit, "limit", 5, "Max results (search) or entries per type (list)")
	yaadCmd.AddCommand(yaadSearchCmd)
	rootCmd.AddCommand(yaadCmd)
}
