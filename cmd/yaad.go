package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/spf13/cobra"
)

var yaadLimit int

var yaadCmd = &cobra.Command{
	Use:   "yaad",
	Short: "Show yaad persistent memory graph",
	Long:  "Print yaad memory counts and recent entries. Hawk embeds yaad as a library at ~/.yaad/data/yaad.db — no separate daemon required.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if yaadLimit < 1 {
			return fmt.Errorf("--limit must be at least 1")
		}
		cmd.Println(memory.FormatYaadDetail(yaadLimit))
		return nil
	},
}

func init() {
	yaadCmd.Flags().IntVar(&yaadLimit, "limit", 5, "Max memories to show per node type")
	rootCmd.AddCommand(yaadCmd)
}
