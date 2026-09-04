package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/session"
	"github.com/spf13/cobra"
)

var (
	sessionMigrateID         string
	sessionMigrateAllowLarge bool
	sessionMigrateJSON       bool
)

var sessionMigrateCmd = &cobra.Command{
	Use:   "migrate <id>|--id <id>",
	Short: "Migrate a saved session to the current format (fx session migrate parity)",
	Long: `Upgrade a saved session to the current on-disk JSONL format, mirroring fx's
"session migrate" command.

Legacy .json sessions are loaded and re-persisted in the current format;
.jsonl sessions have their format_version header bumped. Oversized sessions are
refused unless --allow-large is set.

Use --json to emit the migration result machine-readably.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionMigrate,
}

func init() {
	sessionMigrateCmd.Flags().StringVar(&sessionMigrateID, "id", "", "session id to migrate")
	sessionMigrateCmd.Flags().BoolVar(&sessionMigrateAllowLarge, "allow-large", false, "permit migrating an oversized session")
	sessionMigrateCmd.Flags().BoolVar(&sessionMigrateJSON, "json", false, "output the result as JSON")
	sessionsCmd.AddCommand(sessionMigrateCmd)
}

func runSessionMigrate(cmd *cobra.Command, args []string) error {
	id := strings.TrimSpace(sessionMigrateID)
	if len(args) > 0 {
		id = strings.TrimSpace(args[0])
	}
	if id == "" {
		return fmt.Errorf("session migrate: specify a session id as an argument or with --id")
	}

	res, err := session.MigrateSession(id, sessionMigrateAllowLarge)
	if err != nil {
		return err
	}

	if sessionMigrateJSON {
		raw, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("session migrate: marshal json: %w", err)
		}
		_, _ = cmd.OutOrStdout().Write(raw)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}

	if res.FromVersion >= res.ToVersion {
		cmd.Println(fmt.Sprintf("Session %s is already at the current format (v%d).", res.ID, res.ToVersion))
	} else {
		cmd.Println(fmt.Sprintf("Migrated session %s from v%d to v%d (%d bytes).", res.ID, res.FromVersion, res.ToVersion, res.SizeBytes))
	}
	return nil
}
