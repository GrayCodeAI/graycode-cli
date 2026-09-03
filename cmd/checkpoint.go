package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/session"
	"github.com/spf13/cobra"
)

// checkpointCmd groups named session-checkpoint operations: snapshot the current
// (or latest) session under a label, list snapshots, restore one, or delete one.
// This is additive on top of `--resume <id>`; named checkpoints let you save a
// labeled point you can come back to with `graycode resume <name>`.
var checkpointCmd = &cobra.Command{
	Use:   "checkpoint",
	Short: "Save and restore named session checkpoints",
	Long: `checkpoint snapshots a session under a human-friendly label so you can
return to it later with "graycode resume <name>".

Subcommands:
  save <name>      Snapshot the latest session in this directory under <name>
  list             List all named checkpoints
  restore <name>   Restore a named checkpoint into a resumable session
  delete <name>    Delete a named checkpoint`,
}

var checkpointSaveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Snapshot the latest session under a label",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		var s *session.Session
		var err error
		if checkpointSessionID != "" {
			s, err = session.Load(checkpointSessionID)
		} else {
			s, err = session.LoadLatestForCWD("")
		}
		if err != nil || s == nil {
			return fmt.Errorf("no session to checkpoint: %w", err)
		}

		cp, err := session.SaveNamedCheckpoint(name, s)
		if err != nil {
			return err
		}
		cmd.Printf("Saved checkpoint %q (session %s, %d messages, %s/%s)\n",
			cp.Name, cp.Session.ID, len(cp.Session.Messages), cp.Session.Provider, cp.Session.Model)
		cmd.Printf("Resume with: graycode resume %s\n", name)
		return nil
	},
}

var checkpointListJSON bool

var checkpointListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all named checkpoints",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cps, err := session.ListNamedCheckpoints()
		if err != nil {
			return err
		}
		if checkpointListJSON {
			out, err := json.MarshalIndent(cps, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling checkpoints: %w", err)
			}
			cmd.Println(string(out))
			return nil
		}
		if len(cps) == 0 {
			cmd.Println("No named checkpoints.")
			return nil
		}
		cmd.Printf("Named checkpoints (%d):\n", len(cps))
		now := time.Now()
		for _, cp := range cps {
			age := now.Sub(cp.CreatedAt).Round(time.Second)
			msgs := 0
			if cp.Session != nil {
				msgs = len(cp.Session.Messages)
			}
			cmd.Printf("  %-20s  %d msgs  (%s ago)\n", cp.Name, msgs, age)
		}
		return nil
	},
}

var checkpointRestoreCmd = &cobra.Command{
	Use:   "restore <name>",
	Short: "Restore a named checkpoint into a resumable session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return restoreNamedCheckpoint(cmd, args[0])
	},
}

var checkpointDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a named checkpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := session.DeleteNamedCheckpoint(args[0]); err != nil {
			return err
		}
		cmd.Printf("Deleted checkpoint %q\n", args[0])
		return nil
	},
}

// resumeCmd is a top-level convenience for `graycode resume <name>`: it restores a
// named checkpoint into a session file and tells the user how to continue it.
var resumeCmd = &cobra.Command{
	Use:   "resume <name>",
	Short: "Restore a named session checkpoint and resume it",
	Long: `resume restores a session previously saved with "graycode checkpoint save"
into a resumable session, then prints the command to continue the conversation.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return restoreNamedCheckpoint(cmd, args[0])
	},
}

var checkpointSessionID string

// restoreNamedCheckpoint writes a named checkpoint's snapshot back into the
// session store so the normal chat flow can pick it up via `--resume <id>`.
func restoreNamedCheckpoint(cmd *cobra.Command, name string) error {
	cp, err := session.LoadNamedCheckpoint(name)
	if err != nil {
		return err
	}
	if cp.Session == nil {
		return fmt.Errorf("checkpoint %q has no session data", name)
	}
	if err := session.Save(cp.Session); err != nil {
		return fmt.Errorf("restore session: %w", err)
	}
	cmd.Printf("Restored checkpoint %q into session %s (%d messages, %s/%s)\n",
		cp.Name, cp.Session.ID, len(cp.Session.Messages), cp.Session.Provider, cp.Session.Model)
	cmd.Printf("Continue with: graycode --resume %s\n", cp.Session.ID)
	return nil
}

func init() {
	checkpointSaveCmd.Flags().StringVar(&checkpointSessionID, "session-id", "", "checkpoint a specific session ID instead of the latest")
	checkpointListCmd.Flags().BoolVar(&checkpointListJSON, "json", false, "output checkpoints as JSON")
	checkpointCmd.AddCommand(checkpointSaveCmd)
	checkpointCmd.AddCommand(checkpointListCmd)
	checkpointCmd.AddCommand(checkpointRestoreCmd)
	checkpointCmd.AddCommand(checkpointDeleteCmd)
	rootCmd.AddCommand(checkpointCmd)
	rootCmd.AddCommand(resumeCmd)
}
