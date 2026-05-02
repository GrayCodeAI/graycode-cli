package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/diffsandbox"
	"github.com/spf13/cobra"
)

// sandboxInstance is a package-level sandbox for the CLI session.
// In a real integration this would be loaded/shared from the hawk engine;
// for now we create a fresh sandbox rooted at the current directory.
var sandboxInstance *diffsandbox.Sandbox

func getSandbox() *diffsandbox.Sandbox {
	if sandboxInstance == nil {
		sandboxInstance = diffsandbox.New(".")
	}
	return sandboxInstance
}

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "View, apply, or discard pending diff sandbox changes",
	Long: `sandbox manages the diff sandbox which accumulates proposed file
changes without modifying the filesystem until explicitly applied.

Subcommands:
  status    Show a summary of pending changes
  diff      Show the unified diff of all pending changes
  apply     Apply all pending changes to disk
  discard   Discard all pending changes`,
}

var sandboxStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a summary of pending changes",
	Run: func(cmd *cobra.Command, args []string) {
		sb := getSandbox()
		cmd.Println(sb.Summary())
	},
}

var sandboxDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show the unified diff of all pending changes",
	Run: func(cmd *cobra.Command, args []string) {
		sb := getSandbox()
		d := sb.Diff()
		if d == "" {
			cmd.Println("No pending changes.")
			return
		}
		fmt.Print(d)
	},
}

var sandboxApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply all pending changes to disk",
	RunE: func(cmd *cobra.Command, args []string) error {
		sb := getSandbox()
		if !sb.HasChanges() {
			cmd.Println("No pending changes to apply.")
			return nil
		}

		stats := sb.Stats()
		cmd.Println(fmt.Sprintf("Applying %d change(s): +%d -%d lines, %d created, %d modified, %d deleted",
			stats.FilesCreated+stats.FilesModified+stats.FilesDeleted,
			stats.LinesAdded, stats.LinesRemoved,
			stats.FilesCreated, stats.FilesModified, stats.FilesDeleted))

		if err := sb.Apply(); err != nil {
			return fmt.Errorf("apply failed: %w", err)
		}

		cmd.Println("All changes applied.")
		return nil
	},
}

var sandboxDiscardCmd = &cobra.Command{
	Use:   "discard",
	Short: "Discard all pending changes",
	Run: func(cmd *cobra.Command, args []string) {
		sb := getSandbox()
		if !sb.HasChanges() {
			cmd.Println("No pending changes to discard.")
			return
		}
		sb.Discard()
		cmd.Println("All pending changes discarded.")
	},
}

func init() {
	sandboxCmd.AddCommand(sandboxStatusCmd)
	sandboxCmd.AddCommand(sandboxDiffCmd)
	sandboxCmd.AddCommand(sandboxApplyCmd)
	sandboxCmd.AddCommand(sandboxDiscardCmd)
}
