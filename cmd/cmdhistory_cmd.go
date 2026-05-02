package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/GrayCodeAI/hawk/cmdhistory"
	"github.com/spf13/cobra"
)

var (
	cmdHistoryLimit    int
	cmdHistoryCWD      string
	cmdHistoryExitCode int
	cmdHistoryExitSet  bool
)

var cmdHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Search and browse command history",
	Long: `history provides access to the structured command history database,
recording every Bash tool call with context (exit code, duration, cwd, git branch).

Subcommands:
  search <query>   Full-text search across command history
  recent [N]       Show N most recent commands (default 20)
  stats            Show aggregate usage statistics`,
}

var cmdHistorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search across command history",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openCmdHistoryStore()
		if err != nil {
			return err
		}
		defer store.Close()

		query := args[0]
		for _, a := range args[1:] {
			query += " " + a
		}

		opts := cmdhistory.SearchOpts{
			Limit: cmdHistoryLimit,
			CWD:   cmdHistoryCWD,
		}
		if cmdHistoryExitSet {
			opts.ExitCode = &cmdHistoryExitCode
		}

		entries, err := store.Search(query, opts)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(entries) == 0 {
			cmd.Println("No matching commands found.")
			return nil
		}

		for _, e := range entries {
			printCmdHistoryEntry(cmd, e)
		}
		return nil
	},
}

var cmdHistoryRecentCmd = &cobra.Command{
	Use:   "recent [N]",
	Short: "Show N most recent commands (default 20)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openCmdHistoryStore()
		if err != nil {
			return err
		}
		defer store.Close()

		n := 20
		if len(args) > 0 {
			parsed, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid number: %s", args[0])
			}
			n = parsed
		}

		entries, err := store.Recent(n)
		if err != nil {
			return fmt.Errorf("recent query failed: %w", err)
		}

		if len(entries) == 0 {
			cmd.Println("No command history found.")
			return nil
		}

		for _, e := range entries {
			printCmdHistoryEntry(cmd, e)
		}
		return nil
	},
}

var cmdHistoryStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show aggregate usage statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openCmdHistoryStore()
		if err != nil {
			return err
		}
		defer store.Close()

		stats, err := store.Stats()
		if err != nil {
			return fmt.Errorf("stats query failed: %w", err)
		}

		cmd.Println(fmt.Sprintf("Total commands:  %d", stats.TotalCommands))
		cmd.Println(fmt.Sprintf("Unique commands: %d", stats.UniqueCommands))
		cmd.Println(fmt.Sprintf("Success rate:    %.1f%%", stats.SuccessRate*100))
		cmd.Println()

		if len(stats.TopCommands) > 0 {
			cmd.Println("Top commands:")
			for _, tc := range stats.TopCommands {
				cmd.Println(fmt.Sprintf("  %4d  %s", tc.Count, tc.Command))
			}
			cmd.Println()
		}

		if len(stats.TopDirectories) > 0 {
			cmd.Println("Top directories:")
			for _, td := range stats.TopDirectories {
				cmd.Println(fmt.Sprintf("  %4d  %s", td.Count, td.Dir))
			}
		}

		return nil
	},
}

func init() {
	cmdHistorySearchCmd.Flags().IntVar(&cmdHistoryLimit, "limit", 50, "maximum number of results")
	cmdHistorySearchCmd.Flags().StringVar(&cmdHistoryCWD, "cwd", "", "filter by working directory")
	cmdHistorySearchCmd.Flags().IntVar(&cmdHistoryExitCode, "exit-code", 0, "filter by exit code")
	cmdHistorySearchCmd.Flags().BoolVar(&cmdHistoryExitSet, "filter-exit-code", false, "enable exit code filter")

	cmdHistoryCmd.AddCommand(cmdHistorySearchCmd)
	cmdHistoryCmd.AddCommand(cmdHistoryRecentCmd)
	cmdHistoryCmd.AddCommand(cmdHistoryStatsCmd)
}

func openCmdHistoryStore() (*cmdhistory.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	dbPath := filepath.Join(home, ".hawk", "cmd-history.db")

	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("cannot create history directory: %w", err)
	}

	store, err := cmdhistory.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	return store, nil
}

func printCmdHistoryEntry(cmd *cobra.Command, e cmdhistory.Entry) {
	exitLabel := "ok"
	if e.ExitCode != 0 {
		exitLabel = fmt.Sprintf("exit:%d", e.ExitCode)
	}
	cmd.Println(fmt.Sprintf("[%s] [%s] [%s] %s",
		e.CreatedAt.Format("2006-01-02 15:04:05"),
		exitLabel,
		e.Duration.Round(1),
		e.Command,
	))
	if e.CWD != "" {
		cmd.Println(fmt.Sprintf("  cwd: %s", e.CWD))
	}
	if e.GitBranch != "" {
		cmd.Println(fmt.Sprintf("  branch: %s", e.GitBranch))
	}
}
